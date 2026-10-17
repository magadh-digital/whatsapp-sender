// Package chatmsg: free-form WhatsApp replies for Magadh Bot, so the bot never holds the Meta token.
//
//	POST /chat-message   one of: text, voice (audio_base64 | audio_url), reaction, mark_read (+ typing)
//	GET  /media/:id      an inbound media file's bytes (Meta media id → URL → download with the token)
//
// Auth = the existing service key NOTIFY_STATUS_SECRET (the one mds-orders signs POST /meta-status with), same
// header and scheme: X-Notify-Signature: sha256=<hex HMAC-SHA256>. Signed bytes: the exact body for POST, the string
// "GET " + request URI for GET. Each request carries ts (unix seconds; the body's "ts", the query's "ts" for GET)
// that must be within 5 minutes, and a signature is accepted once (Redis). No key set = 503 for everything.
//
// Sends are synchronous (not the template queue) and answer with what Meta said, so the caller can tell
// "not sent" from "unknown":
//
//	200 {ok:true, outcome:"sent", wamid}          Meta took it (mark_read: {ok:true, outcome:"sent"})
//	400 {ok:false, outcome:"not_sent", error}     refused here (validation)
//	502 {ok:false, outcome:"not_sent", meta_status, meta_code, meta_subcode, error}   Meta refused it (4xx), or we
//	                                              never reached Meta, or the voice upload failed (nothing was sent)
//	504 {ok:false, outcome:"unknown", error}      no answer / 5xx / an answer without a message id: it may have gone
//
// Text and voice sends are logged to message_logs (template chat_text / chat_voice) with Meta's answer; the text
// itself is never stored (Masked + its length).
package chatmsg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"notify/config"
	"notify/handler/notifylogs"
	"notify/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	TextLimit     = 4096             // Meta's text body limit (UTF-16 units, as the bot splits)
	MaxMediaBytes = 16 << 20         // Meta's audio limit; also the GET /media cap
	maxBodyBytes  = 23 << 20         // base64 of 16 MB + JSON
	MaxSkew       = 5 * time.Minute  // ts window
	replayTTL     = 15 * time.Minute // signature remembered for longer than the window
)

var (
	mediaIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)
	wamidRe   = regexp.MustCompile(`^[A-Za-z0-9._=+/:-]{1,256}$`)
	nonDigit  = regexp.MustCompile(`\D`)
	// Meta's accepted audio types (ogg = opus only)
	audioMimes = map[string]bool{"audio/ogg": true, "audio/mpeg": true, "audio/mp4": true, "audio/aac": true, "audio/amr": true}
	metaHostRe = regexp.MustCompile(`(?i)(^|\.)(fbsbx\.com|facebook\.com|whatsapp\.net|fbcdn\.net)$`)
)

// Deps: everything the handlers touch, so tests can swap Meta, Redis and Mongo.
type Deps struct {
	Cfg config.ChatConfig
	// Meta Graph calls (token-bearing)
	Meta *http.Client
	// audio_url downloads (allowlisted hosts, no redirects)
	Fetch *http.Client
	// FirstUse records a signature; false = seen before (a replay)
	FirstUse func(key string, ttl time.Duration) (bool, error)
	// Log stores one send (nil = not logged)
	Log func(*models.MessageLog)
	Now func() time.Time
}

func noRedirect(*http.Request, []*http.Request) error { return errors.New("redirects not followed") }

// NewClients: Meta 30 s, audio fetch 20 s; neither follows redirects (the token must not travel).
func NewClients() (*http.Client, *http.Client) {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirect}, &http.Client{Timeout: 20 * time.Second, CheckRedirect: noRedirect}
}

func Register(r gin.IRoutes, d *Deps) {
	r.POST("/chat-message", d.ChatMessage)
	r.GET("/media/:id", d.Media)
}

// ---------------------------------------------------------------- auth

// verify: key set, signature over msg, ts fresh, signature not used before. Returns status + message on refusal.
func (d *Deps) verify(msg []byte, header string, ts int64) (int, string) {
	if d.Cfg.Secret == "" {
		return 503, "Chat sends off: set NOTIFY_STATUS_SECRET"
	}
	if !notifylogs.SignatureOK(d.Cfg.Secret, msg, header) {
		return 401, "Bad signature"
	}
	now := d.Now()
	if ts <= 0 || absDur(now.Sub(time.Unix(ts, 0))) > MaxSkew {
		return 401, "Stale or missing ts"
	}
	if d.FirstUse != nil {
		sum := sha256.Sum256([]byte(header))
		first, err := d.FirstUse(hex.EncodeToString(sum[:]), replayTTL)
		if err != nil {
			return 503, "Replay guard unavailable"
		}
		if !first {
			return 401, "Replayed request"
		}
	}
	return 0, ""
}

func absDur(x time.Duration) time.Duration {
	if x < 0 {
		return -x
	}
	return x
}

// ---------------------------------------------------------------- validation

// NormalizePhone: as the template sends ("91" + phone): 10 digits, or 91 + 10 digits, any punctuation.
// Returns the 10-digit number (as message_logs stores it) and the wa id (91…), or ok=false.
func NormalizePhone(p string) (string, string, bool) {
	d := nonDigit.ReplaceAllString(p, "")
	if len(d) == 12 && strings.HasPrefix(d, "91") {
		d = d[2:]
	}
	if len(d) != 10 {
		return "", "", false
	}
	return d, "91" + d, true
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

type Reaction struct {
	MessageID string  `json:"message_id"`
	Emoji     *string `json:"emoji"`
}

type ChatRequest struct {
	Ts          int64     `json:"ts"`
	Phone       string    `json:"phone"`
	Text        *string   `json:"text"`
	AudioURL    string    `json:"audio_url"`
	AudioBase64 string    `json:"audio_base64"`
	Mime        string    `json:"mime"`
	ReplyTo     string    `json:"reply_to"`
	MarkRead    string    `json:"mark_read"`
	Typing      bool      `json:"typing"`
	Reaction    *Reaction `json:"reaction"`
}

// Kind: "text" | "voice" | "reaction" | "mark_read", or an error text when the body is not exactly one of them.
func (r *ChatRequest) Kind() (string, string) {
	kinds := []string{}
	if r.Text != nil {
		kinds = append(kinds, "text")
	}
	if r.AudioURL != "" || r.AudioBase64 != "" {
		kinds = append(kinds, "voice")
	}
	if r.Reaction != nil {
		kinds = append(kinds, "reaction")
	}
	if r.MarkRead != "" {
		kinds = append(kinds, "mark_read")
	}
	if len(kinds) != 1 {
		return "", "Send exactly one of text, audio_base64 / audio_url, reaction, mark_read"
	}
	k := kinds[0]
	if r.AudioURL != "" && r.AudioBase64 != "" {
		return "", "audio_url or audio_base64, not both"
	}
	if r.ReplyTo != "" && (k == "mark_read" || k == "reaction" || !wamidRe.MatchString(r.ReplyTo)) {
		return "", "reply_to must be a message id, with text or voice only"
	}
	switch k {
	case "text":
		t := *r.Text
		if strings.TrimSpace(t) == "" {
			return "", "text is empty"
		}
		if !utf8.ValidString(t) {
			return "", "text is not UTF-8"
		}
		if n := utf16Len(t); n > TextLimit {
			return "", fmt.Sprintf("text is %d characters; at most %d (split it)", n, TextLimit)
		}
	case "reaction":
		if !wamidRe.MatchString(r.Reaction.MessageID) {
			return "", "reaction.message_id must be a message id"
		}
		if r.Reaction.Emoji == nil || utf8.RuneCountInString(*r.Reaction.Emoji) > 8 || strings.ContainsAny(*r.Reaction.Emoji, "<>\"\\\n") {
			return "", `reaction.emoji: one emoji, or "" to remove the reaction`
		}
	case "mark_read":
		if !wamidRe.MatchString(r.MarkRead) {
			return "", "mark_read must be a message id"
		}
	}
	return k, ""
}

// AudioMime: the bare type (parameters such as codecs=opus dropped) when Meta accepts it, else "".
func AudioMime(m string) string {
	t, _, err := mime.ParseMediaType(strings.TrimSpace(m))
	if err != nil {
		return ""
	}
	t = strings.ToLower(t)
	if audioMimes[t] {
		return t
	}
	return ""
}

// AudioURLAllowed: http(s), no credentials, host (or host:port) on the allowlist exactly.
func AudioURLAllowed(raw string, hosts []string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Host == "" {
		return false
	}
	h := strings.ToLower(u.Host)
	for _, a := range hosts {
		if a == h || a == strings.ToLower(u.Hostname()) && u.Port() == "" {
			return true
		}
	}
	return false
}

// MediaURLAllowed: where Meta's media URL may take the token: Meta's hosts over https, or the configured Graph
// origin (the dev stub).
func MediaURLAllowed(raw, graphBase string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" {
		return false
	}
	if g, err := url.Parse(graphBase); err == nil && g.Scheme == u.Scheme && strings.EqualFold(g.Host, u.Host) {
		return true
	}
	return u.Scheme == "https" && metaHostRe.MatchString(u.Hostname())
}

// ---------------------------------------------------------------- Meta calls

// Result: what one send came to.
type Result struct {
	Outcome     string         // sent | not_sent | unknown
	HTTP        int            // our answer's status
	Wamid       string         // Meta's message id
	MetaStatus  int            // Meta's HTTP status (0 = no answer)
	MetaCode    int            // error.code
	MetaSubcode int            // error.error_subcode
	Error       string         // for the caller
	Payload     map[string]any // Meta's answer (logged)
}

func (r Result) JSON() gin.H {
	h := gin.H{"ok": r.Outcome == "sent", "outcome": r.Outcome}
	if r.Wamid != "" {
		h["wamid"] = r.Wamid
	}
	if r.MetaStatus != 0 {
		h["meta_status"] = r.MetaStatus
	}
	if r.MetaCode != 0 {
		h["meta_code"] = r.MetaCode
	}
	if r.MetaSubcode != 0 {
		h["meta_subcode"] = r.MetaSubcode
	}
	if r.Error != "" {
		h["error"] = r.Error
	}
	return h
}

func notSent(status int, msg string) Result {
	return Result{Outcome: "not_sent", HTTP: status, Error: msg}
}

// transportResult: a request that got no answer. Never connected (dial / DNS / refused) = not sent; anything later
// (timeout while waiting, connection dropped) = unknown.
func transportResult(err error) Result {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return Result{Outcome: "not_sent", HTTP: 502, Error: "Meta unreachable: " + err.Error()}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return Result{Outcome: "not_sent", HTTP: 502, Error: "Meta unreachable: " + err.Error()}
	}
	return Result{Outcome: "unknown", HTTP: 504, Error: "No answer from Meta: " + err.Error()}
}

type metaErr struct {
	Message      string `json:"message"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	UserTitle    string `json:"error_user_title"`
	UserMsg      string `json:"error_user_msg"`
	ErrorData    struct {
		Details string `json:"details"`
	} `json:"error_data"`
}

// MapMetaAnswer turns a Graph answer into a Result. needID: a message send must come back with messages[0].id.
func MapMetaAnswer(status int, raw []byte, needID bool) Result {
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if status >= 200 && status <= 299 {
		res := Result{Outcome: "sent", HTTP: 200, MetaStatus: status, Payload: body}
		if !needID {
			return res
		}
		if msgs, _ := body["messages"].([]any); len(msgs) > 0 {
			if m, _ := msgs[0].(map[string]any); m != nil {
				if id, _ := m["id"].(string); id != "" {
					res.Wamid = id
					return res
				}
			}
		}
		return Result{Outcome: "unknown", HTTP: 504, MetaStatus: status, Payload: body, Error: "Meta answered without a message id"}
	}
	var wrap struct {
		Error metaErr `json:"error"`
	}
	_ = json.Unmarshal(raw, &wrap)
	e := wrap.Error
	msg := e.Message
	if e.ErrorData.Details != "" && e.ErrorData.Details != msg {
		msg = strings.TrimSpace(msg + ": " + e.ErrorData.Details)
	}
	if msg == "" {
		msg = fmt.Sprintf("Meta answered HTTP %d", status)
	}
	res := Result{HTTP: 502, MetaStatus: status, MetaCode: e.Code, MetaSubcode: e.ErrorSubcode, Error: msg, Payload: body, Outcome: "not_sent"}
	if status >= 500 {
		res.Outcome, res.HTTP = "unknown", 504
	}
	return res
}

func (d *Deps) graphURL(path string) string {
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(d.Cfg.GraphBaseURL, "/"), d.Cfg.GraphVersion, strings.TrimLeft(path, "/"))
}

func (d *Deps) postMessage(payload map[string]any, needID bool) Result {
	payload["messaging_product"] = "whatsapp"
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", d.graphURL(url.PathEscape(d.Cfg.PhoneNumberID)+"/messages"), bytes.NewReader(b))
	if err != nil {
		return notSent(500, err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+d.Cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.Meta.Do(req)
	if err != nil {
		return transportResult(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{Outcome: "unknown", HTTP: 504, MetaStatus: resp.StatusCode, Error: "Meta answer unreadable: " + err.Error()}
	}
	return MapMetaAnswer(resp.StatusCode, raw, needID)
}

// uploadMedia: POST /{pnid}/media (multipart). Any failure = nothing was sent.
func (d *Deps) uploadMedia(data []byte, mimeType string) (string, Result) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("messaging_product", "whatsapp")
	_ = w.WriteField("type", mimeType)
	h := make(textproto.MIMEHeader)
	ext := map[string]string{"audio/ogg": "ogg", "audio/mpeg": "mp3", "audio/mp4": "m4a", "audio/aac": "aac", "audio/amr": "amr"}[mimeType]
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="voice.%s"`, ext))
	h.Set("Content-Type", mimeType)
	part, _ := w.CreatePart(h)
	_, _ = part.Write(data)
	_ = w.Close()
	req, err := http.NewRequest("POST", d.graphURL(url.PathEscape(d.Cfg.PhoneNumberID)+"/media"), &buf)
	if err != nil {
		return "", notSent(500, err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+d.Cfg.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := d.Meta.Do(req)
	if err != nil {
		return "", notSent(502, "Voice upload failed: "+err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		r := MapMetaAnswer(resp.StatusCode, raw, false)
		r.Outcome, r.HTTP, r.Error = "not_sent", 502, "Voice upload refused: "+r.Error
		return "", r
	}
	var up struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &up)
	if up.ID == "" {
		return "", notSent(502, "Voice upload returned no media id")
	}
	return up.ID, Result{}
}

// readCapped reads at most max bytes; more = errTooBig.
var errTooBig = errors.New("too big")

func readCapped(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errTooBig
	}
	return b, nil
}

func (d *Deps) fetchAudio(raw string) ([]byte, string, Result) {
	if !AudioURLAllowed(raw, d.Cfg.AudioURLHosts) {
		return nil, "", notSent(400, "audio_url host not allowed")
	}
	resp, err := d.Fetch.Get(raw)
	if err != nil {
		return nil, "", notSent(502, "audio_url download failed: "+err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", notSent(502, fmt.Sprintf("audio_url answered HTTP %d", resp.StatusCode))
	}
	if resp.ContentLength > MaxMediaBytes {
		return nil, "", notSent(413, "audio larger than 16 MB")
	}
	b, err := readCapped(resp.Body, MaxMediaBytes)
	if err == errTooBig {
		return nil, "", notSent(413, "audio larger than 16 MB")
	}
	if err != nil {
		return nil, "", notSent(502, "audio_url download failed: "+err.Error())
	}
	return b, resp.Header.Get("Content-Type"), Result{}
}

// ---------------------------------------------------------------- handlers

func (d *Deps) ChatMessage(ctx *gin.Context) {
	raw, err := readCapped(ctx.Request.Body, maxBodyBytes)
	if err == errTooBig {
		ctx.JSON(413, notSent(413, "Body too large").JSON())
		return
	}
	if err != nil {
		ctx.JSON(400, notSent(400, "Unreadable body").JSON())
		return
	}
	var in ChatRequest
	jsonErr := json.Unmarshal(raw, &in)
	// the signature is checked before anything in the body is trusted (ts comes from the signed bytes)
	if code, msg := d.verify(raw, ctx.GetHeader("X-Notify-Signature"), in.Ts); code != 0 {
		ctx.JSON(code, notSent(code, msg).JSON())
		return
	}
	if jsonErr != nil {
		ctx.JSON(400, notSent(400, "Unreadable JSON: "+jsonErr.Error()).JSON())
		return
	}
	if d.Cfg.Token == "" || d.Cfg.PhoneNumberID == "" {
		ctx.JSON(503, notSent(503, "WhatsApp sending not configured: WHATSAPP_AUTH and WHATSAPP_PHONE_NUMBER_ID (or WHATSAPP_URL …/<id>/messages)").JSON())
		return
	}
	kind, bad := in.Kind()
	if bad != "" {
		ctx.JSON(400, notSent(400, bad).JSON())
		return
	}
	if kind == "mark_read" {
		p := map[string]any{"status": "read", "message_id": in.MarkRead}
		if in.Typing {
			p["typing_indicator"] = map[string]any{"type": "text"}
		}
		res := d.postMessage(p, false)
		ctx.JSON(res.HTTP, res.JSON())
		return
	}
	phone10, waID, ok := NormalizePhone(in.Phone)
	if !ok {
		ctx.JSON(400, notSent(400, "phone: an Indian number (10 digits, or 91 + 10 digits)").JSON())
		return
	}
	p := map[string]any{"recipient_type": "individual", "to": waID}
	if in.ReplyTo != "" {
		p["context"] = map[string]any{"message_id": in.ReplyTo}
	}
	logBody := bson.M{"type": kind}
	switch kind {
	case "text":
		p["type"] = "text"
		p["text"] = map[string]any{"body": *in.Text, "preview_url": false}
		logBody["text"] = notifylogs.Masked
		logBody["chars"] = utf16Len(*in.Text)
	case "reaction":
		p["type"] = "reaction"
		p["reaction"] = map[string]any{"message_id": in.Reaction.MessageID, "emoji": *in.Reaction.Emoji}
	case "voice":
		var data []byte
		var ctype string
		if in.AudioBase64 != "" {
			b, err := base64.StdEncoding.DecodeString(in.AudioBase64)
			if err != nil {
				ctx.JSON(400, notSent(400, "audio_base64 is not base64").JSON())
				return
			}
			data, ctype = b, in.Mime
		} else {
			b, ct, res := d.fetchAudio(in.AudioURL)
			if res.Outcome != "" {
				ctx.JSON(res.HTTP, res.JSON())
				return
			}
			data, ctype = b, in.Mime
			if ctype == "" {
				ctype = ct
			}
		}
		if len(data) == 0 {
			ctx.JSON(400, notSent(400, "audio is empty").JSON())
			return
		}
		if len(data) > MaxMediaBytes {
			ctx.JSON(413, notSent(413, "audio larger than 16 MB").JSON())
			return
		}
		mt := AudioMime(ctype)
		if mt == "" {
			ctx.JSON(400, notSent(400, "mime: audio/ogg (opus), audio/mpeg, audio/mp4, audio/aac or audio/amr").JSON())
			return
		}
		id, res := d.uploadMedia(data, mt)
		if res.Outcome != "" {
			d.log("chat_voice", res, phone10, logBody)
			ctx.JSON(res.HTTP, res.JSON())
			return
		}
		p["type"] = "audio"
		p["audio"] = map[string]any{"id": id}
		logBody["bytes"] = len(data)
		logBody["mime"] = mt
	}
	res := d.postMessage(p, true)
	if kind != "reaction" {
		d.log("chat_"+kind, res, phone10, logBody)
	}
	ctx.JSON(res.HTTP, res.JSON())
}

func (d *Deps) log(template string, res Result, phone string, body bson.M) {
	if d.Log == nil {
		return
	}
	payload := res.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	status := models.MessageLogStatusSuccess
	if res.Outcome != "sent" {
		status = models.MessageLogStatusFailed
		if _, has := payload["error"]; !has {
			payload["error"] = map[string]any{"message": res.Error}
		}
		payload["outcome"] = res.Outcome
	}
	d.Log(models.NewMessageLog(template, payload, phone, status, body))
}

// Media: GET /media/:id?ts=… → the file's bytes (Content-Type from Meta), at most 16 MB.
func (d *Deps) Media(ctx *gin.Context) {
	ts, _ := strconv.ParseInt(ctx.Query("ts"), 10, 64)
	if code, msg := d.verify([]byte("GET "+ctx.Request.URL.RequestURI()), ctx.GetHeader("X-Notify-Signature"), ts); code != 0 {
		ctx.JSON(code, gin.H{"ok": false, "error": msg})
		return
	}
	id := ctx.Param("id")
	if len(id) > 128 || !mediaIDRe.MatchString(id) {
		ctx.JSON(400, gin.H{"ok": false, "error": "Bad media id"})
		return
	}
	if d.Cfg.Token == "" {
		ctx.JSON(503, gin.H{"ok": false, "error": "WHATSAPP_AUTH not set"})
		return
	}
	req, _ := http.NewRequest("GET", d.graphURL(url.PathEscape(id)), nil)
	req.Header.Set("Authorization", "Bearer "+d.Cfg.Token)
	resp, err := d.Meta.Do(req)
	if err != nil {
		ctx.JSON(502, gin.H{"ok": false, "error": "Meta unreachable: " + err.Error()})
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode != 200 {
		r := MapMetaAnswer(resp.StatusCode, raw, false)
		status := 502
		if resp.StatusCode == 404 || r.MetaCode == 100 {
			status = 404
		}
		ctx.JSON(status, gin.H{"ok": false, "error": r.Error, "meta_status": r.MetaStatus, "meta_code": r.MetaCode})
		return
	}
	var meta struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		FileSize int64  `json:"file_size"`
	}
	_ = json.Unmarshal(raw, &meta)
	if meta.FileSize > MaxMediaBytes {
		ctx.JSON(413, gin.H{"ok": false, "error": "Media larger than 16 MB"})
		return
	}
	if meta.URL == "" || !MediaURLAllowed(meta.URL, d.Cfg.GraphBaseURL) {
		ctx.JSON(502, gin.H{"ok": false, "error": "Media URL not on a Meta host"})
		return
	}
	dl, _ := http.NewRequestWithContext(context.Background(), "GET", meta.URL, nil)
	dl.Header.Set("Authorization", "Bearer "+d.Cfg.Token)
	r2, err := d.Meta.Do(dl)
	if err != nil {
		ctx.JSON(502, gin.H{"ok": false, "error": "Media download failed: " + err.Error()})
		return
	}
	defer r2.Body.Close()
	if r2.StatusCode != 200 {
		ctx.JSON(502, gin.H{"ok": false, "error": fmt.Sprintf("Media download: HTTP %d", r2.StatusCode)})
		return
	}
	if r2.ContentLength > MaxMediaBytes {
		ctx.JSON(413, gin.H{"ok": false, "error": "Media larger than 16 MB"})
		return
	}
	data, err := readCapped(r2.Body, MaxMediaBytes)
	if err == errTooBig {
		ctx.JSON(413, gin.H{"ok": false, "error": "Media larger than 16 MB"})
		return
	}
	if err != nil {
		ctx.JSON(502, gin.H{"ok": false, "error": "Media download failed: " + err.Error()})
		return
	}
	ct := meta.MimeType
	if ct == "" {
		ct = r2.Header.Get("Content-Type")
	}
	if _, _, err := mime.ParseMediaType(ct); err != nil || ct == "" {
		ct = "application/octet-stream"
	}
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Header("Cache-Control", "no-store")
	ctx.Data(200, ct, data)
}
