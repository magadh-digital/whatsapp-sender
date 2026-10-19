package chatmsg

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"notify/config"
	"notify/models"

	"github.com/gin-gonic/gin"
)

const secret = "s3cret"

var now = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func sign(msg string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

type fakeMeta struct {
	srv      *httptest.Server
	mu       sync.Mutex
	calls    []string // "METHOD path"
	bodies   []string
	auth     []string
	messages func(body map[string]any) (int, string)
	media    []byte
}

func newFakeMeta(t *testing.T) *fakeMeta {
	f := &fakeMeta{media: []byte("OggS-fake-voice")}
	f.messages = func(map[string]any) (int, string) { return 200, `{"messages":[{"id":"wamid.OUT1"}]}` }
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, string(b))
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/PNID/messages"):
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			st, out := f.messages(m)
			w.WriteHeader(st)
			io.WriteString(w, out)
		case strings.HasSuffix(r.URL.Path, "/PNID/media"):
			if !strings.Contains(string(b), `name="file"`) {
				w.WriteHeader(400)
				return
			}
			io.WriteString(w, `{"id":"MEDIA9"}`)
		case r.URL.Path == "/v21.0/111":
			fmt.Fprintf(w, `{"url":"%s/dl/111","mime_type":"audio/ogg","file_size":%d}`, f.srv.URL, min(len(f.media), 1000))
		case r.URL.Path == "/v21.0/222": // a URL elsewhere: the token must not go there
			io.WriteString(w, `{"url":"http://evil.example/x","mime_type":"audio/ogg"}`)
		case r.URL.Path == "/v21.0/333":
			io.WriteString(w, `{"url":"x","file_size":99999999}`)
		case r.URL.Path == "/dl/111":
			w.Header().Set("Content-Type", "audio/ogg")
			w.Write(f.media)
		default:
			w.WriteHeader(404)
			io.WriteString(w, `{"error":{"message":"Unsupported get request","code":100}}`)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func setup(t *testing.T) (*gin.Engine, *fakeMeta, *[]*models.MessageLog) {
	gin.SetMode(gin.TestMode)
	f := newFakeMeta(t)
	logs := &[]*models.MessageLog{}
	seen := map[string]bool{}
	d := &Deps{
		Cfg:   config.ChatConfig{Secret: secret, Token: "TOKEN", GraphBaseURL: f.srv.URL, GraphVersion: "v21.0", PhoneNumberID: "PNID", AudioURLHosts: []string{strings.TrimPrefix(f.srv.URL, "http://")}},
		Meta:  f.srv.Client(),
		Fetch: f.srv.Client(),
		FirstUse: func(k string, _ time.Duration) (bool, error) {
			if seen[k] {
				return false, nil
			}
			seen[k] = true
			return true, nil
		},
		Log: func(l *models.MessageLog) { *logs = append(*logs, l) },
		Now: func() time.Time { return now },
	}
	r := gin.New()
	Register(r, d)
	return r, f, logs
}

func post(r *gin.Engine, body string, sig string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest("POST", "/chat-message", strings.NewReader(body))
	if sig == "" {
		sig = sign(body)
	}
	req.Header.Set("X-Notify-Signature", sig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func ts() string { return fmt.Sprint(now.Unix()) }

func TestAuth(t *testing.T) {
	r, f, _ := setup(t)
	body := `{"ts":` + ts() + `,"phone":"9431022239","text":"hi"}`
	if w, _ := post(r, body, sign(body+" ")); w.Code != 401 {
		t.Errorf("bad signature: %d", w.Code)
	}
	if w, _ := post(r, body, "sha256=zz"); w.Code != 401 {
		t.Errorf("garbage signature: %d", w.Code)
	}
	stale := `{"ts":` + fmt.Sprint(now.Add(-6*time.Minute).Unix()) + `,"phone":"9431022239","text":"hi"}`
	if w, _ := post(r, stale, ""); w.Code != 401 {
		t.Errorf("stale ts: %d", w.Code)
	}
	if w, _ := post(r, `{"phone":"9431022239","text":"hi"}`, ""); w.Code != 401 {
		t.Errorf("missing ts: %d", w.Code)
	}
	if w, out := post(r, body, ""); w.Code != 200 || out["wamid"] != "wamid.OUT1" || out["outcome"] != "sent" {
		t.Errorf("good request: %d %v", w.Code, out)
	}
	if w, _ := post(r, body, ""); w.Code != 401 {
		t.Errorf("replay accepted: %d", w.Code)
	}
	if len(f.calls) != 1 || f.auth[0] != "Bearer TOKEN" {
		t.Errorf("meta calls %v %v", f.calls, f.auth)
	}
}

func TestNoKeyRefusesAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, &Deps{Cfg: config.ChatConfig{Token: "T", PhoneNumberID: "P"}, Now: func() time.Time { return now }})
	body := `{"ts":` + ts() + `,"phone":"9431022239","text":"hi"}`
	if w, _ := post(r, body, ""); w.Code != 503 {
		t.Errorf("no key: %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/media/111?ts="+ts(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Errorf("media no key: %d", w.Code)
	}
}

func TestValidation(t *testing.T) {
	r, f, _ := setup(t)
	long := strings.Repeat("a", TextLimit+1)
	emoji := strings.Repeat("😀", TextLimit/2+1) // 2 UTF-16 units each
	cases := map[string]string{
		"bad phone":       `"phone":"12345","text":"hi"`,
		"foreign phone":   `"phone":"+1 415 555 0100","text":"hi"`,
		"empty text":      `"phone":"9431022239","text":"  "`,
		"too long":        `"phone":"9431022239","text":"` + long + `"`,
		"too long utf16":  `"phone":"9431022239","text":"` + emoji + `"`,
		"two kinds":       `"phone":"9431022239","text":"hi","mark_read":"wamid.X"`,
		"nothing":         `"phone":"9431022239"`,
		"bad mime":        `"phone":"9431022239","audio_base64":"` + base64.StdEncoding.EncodeToString([]byte("x")) + `","mime":"video/mp4"`,
		"bad base64":      `"phone":"9431022239","audio_base64":"***","mime":"audio/ogg"`,
		"url not allowed": `"phone":"9431022239","audio_url":"http://169.254.169.254/latest","mime":"audio/ogg"`,
		"bad reply_to":    `"phone":"9431022239","text":"hi","reply_to":"../x y"`,
		"bad reaction":    `"phone":"9431022239","reaction":{"message_id":"wamid.X"}`,
		"bad mark_read":   `"mark_read":"a b"`,
	}
	for name, c := range cases {
		w, out := post(r, `{"ts":`+ts()+`,`+c+`}`, "")
		if w.Code != 400 || out["outcome"] != "not_sent" {
			t.Errorf("%s: %d %v", name, w.Code, out)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("Meta called on invalid input: %v", f.calls)
	}
	if w, _ := post(r, `{"ts":`+ts()+`,"phone":"9431022239","text":"`+strings.Repeat("a", TextLimit)+`"}`, ""); w.Code != 200 {
		t.Errorf("4096 refused: %d", w.Code)
	}
}

func TestTextPayloadAndLog(t *testing.T) {
	r, f, logs := setup(t)
	w, _ := post(r, `{"ts":`+ts()+`,"phone":"+91 94310 22239","text":"is mahine 12 visits","reply_to":"wamid.IN1"}`, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(f.bodies[0]), &m)
	if m["to"] != "919431022239" || m["type"] != "text" || m["messaging_product"] != "whatsapp" || m["context"].(map[string]any)["message_id"] != "wamid.IN1" {
		t.Errorf("payload %v", m)
	}
	if len(*logs) != 1 || (*logs)[0].Template != "chat_text" || (*logs)[0].Phone != "9431022239" || (*logs)[0].Status != "SUCCESS" {
		t.Fatalf("log %+v", *logs)
	}
	if b, _ := json.Marshal((*logs)[0].Body); strings.Contains(string(b), "visits") {
		t.Errorf("text stored in the log: %s", b)
	}
}

func TestMetaErrorMapping(t *testing.T) {
	r, f, logs := setup(t)
	f.messages = func(map[string]any) (int, string) {
		return 400, `{"error":{"message":"Re-engagement message","code":131047,"error_subcode":2494010,"error_data":{"details":"More than 24 hours"}}}`
	}
	w, out := post(r, `{"ts":`+ts()+`,"phone":"9431022239","text":"hi"}`, "")
	if w.Code != 502 || out["outcome"] != "not_sent" || out["meta_code"] != float64(131047) || out["meta_status"] != float64(400) || out["meta_subcode"] != float64(2494010) {
		t.Errorf("4xx: %d %v", w.Code, out)
	}
	if (*logs)[0].Status != "FAILED" {
		t.Errorf("log %+v", (*logs)[0])
	}
	f.messages = func(map[string]any) (int, string) {
		return 500, `{"error":{"message":"Something went wrong","code":131000}}`
	}
	if w, out := post(r, `{"ts":`+ts()+`,"phone":"9431022239","text":"hi2"}`, ""); w.Code != 504 || out["outcome"] != "unknown" {
		t.Errorf("5xx: %d %v", w.Code, out)
	}
	f.messages = func(map[string]any) (int, string) { return 200, `{"messages":[]}` }
	if w, out := post(r, `{"ts":`+ts()+`,"phone":"9431022239","text":"hi3"}`, ""); w.Code != 504 || out["outcome"] != "unknown" {
		t.Errorf("no id: %d %v", w.Code, out)
	}
}

func TestTransportOutcomes(t *testing.T) {
	// nothing listens: dial fails = not sent
	srv := httptest.NewServer(http.NotFoundHandler())
	dead := srv.URL
	srv.Close()
	d := &Deps{Cfg: config.ChatConfig{GraphBaseURL: dead, GraphVersion: "v21.0", PhoneNumberID: "PNID", Token: "T"}, Meta: &http.Client{Timeout: time.Second}}
	if res := d.postMessage(map[string]any{}, true); res.Outcome != "not_sent" {
		t.Errorf("dial error: %+v", res)
	}
	// connected, no answer in time = unknown
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(300 * time.Millisecond) }))
	defer slow.Close()
	d.Cfg.GraphBaseURL = slow.URL
	d.Meta = &http.Client{Timeout: 50 * time.Millisecond}
	if res := d.postMessage(map[string]any{}, true); res.Outcome != "unknown" || res.HTTP != 504 {
		t.Errorf("timeout: %+v", res)
	}
}

func TestVoiceUploadThenSend(t *testing.T) {
	r, f, logs := setup(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("OggS....opus"))
	w, out := post(r, `{"ts":`+ts()+`,"phone":"9431022239","audio_base64":"`+b64+`","mime":"audio/ogg; codecs=opus"}`, "")
	if w.Code != 200 || out["wamid"] != "wamid.OUT1" {
		t.Fatalf("%d %v", w.Code, out)
	}
	if len(f.calls) != 2 || !strings.HasSuffix(f.calls[0], "/PNID/media") || !strings.HasSuffix(f.calls[1], "/PNID/messages") {
		t.Fatalf("calls %v", f.calls)
	}
	if !strings.Contains(f.bodies[0], "audio/ogg") || !strings.Contains(f.bodies[1], `"audio":{"id":"MEDIA9"}`) || !strings.Contains(f.bodies[1], `"type":"audio"`) {
		t.Errorf("bodies %v", f.bodies)
	}
	if (*logs)[0].Template != "chat_voice" {
		t.Errorf("log %+v", (*logs)[0])
	}
	// from an allowlisted URL
	w, _ = post(r, `{"ts":`+ts()+`,"phone":"9431022239","audio_url":"`+f.srv.URL+`/dl/111"}`, "")
	if w.Code != 200 {
		t.Errorf("audio_url: %d %s", w.Code, w.Body.String())
	}
}

func TestVoiceSizeCap(t *testing.T) {
	r, f, _ := setup(t)
	big := base64.StdEncoding.EncodeToString(make([]byte, MaxMediaBytes+1))
	w, _ := post(r, `{"ts":`+ts()+`,"phone":"9431022239","audio_base64":"`+big+`","mime":"audio/ogg"}`, "")
	if w.Code != 413 || len(f.calls) != 0 {
		t.Errorf("cap: %d %v", w.Code, f.calls)
	}
}

func TestMarkReadTypingAndReaction(t *testing.T) {
	r, f, logs := setup(t)
	f.messages = func(m map[string]any) (int, string) {
		if m["status"] == "read" {
			return 200, `{"success":true}`
		}
		return 200, `{"messages":[{"id":"wamid.R"}]}`
	}
	if w, out := post(r, `{"ts":`+ts()+`,"mark_read":"wamid.IN1","typing":true}`, ""); w.Code != 200 || out["ok"] != true {
		t.Errorf("mark_read: %d %v", w.Code, out)
	}
	if !strings.Contains(f.bodies[0], `"typing_indicator":{"type":"text"}`) || !strings.Contains(f.bodies[0], `"message_id":"wamid.IN1"`) {
		t.Errorf("mark_read body %s", f.bodies[0])
	}
	if w, out := post(r, `{"ts":`+ts()+`,"phone":"9431022239","reaction":{"message_id":"wamid.IN1","emoji":"👀"}}`, ""); w.Code != 200 || out["wamid"] != "wamid.R" {
		t.Errorf("reaction: %d %v", w.Code, out)
	}
	if !strings.Contains(f.bodies[1], `"type":"reaction"`) || !strings.Contains(f.bodies[1], `"emoji":"👀"`) {
		t.Errorf("reaction body %s", f.bodies[1])
	}
	if len(*logs) != 0 {
		t.Errorf("mark_read / reaction logged: %d", len(*logs))
	}
}

func getMedia(r *gin.Engine, id string, tsv string) *httptest.ResponseRecorder {
	uri := "/media/" + id + "?ts=" + tsv
	req := httptest.NewRequest("GET", uri, nil)
	req.Header.Set("X-Notify-Signature", sign("GET "+uri))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMedia(t *testing.T) {
	r, f, _ := setup(t)
	w := getMedia(r, "111", ts())
	if w.Code != 200 || w.Body.String() != string(f.media) || w.Header().Get("Content-Type") != "audio/ogg" {
		t.Errorf("media: %d %q %s", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
	if w := getMedia(r, "111", ts()); w.Code != 401 {
		t.Errorf("replayed media GET: %d", w.Code)
	}
	if w := getMedia(r, "222", fmt.Sprint(now.Unix()+1)); w.Code != 502 {
		t.Errorf("foreign URL: %d", w.Code)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "evil") {
			t.Error("fetched a foreign URL")
		}
	}
	if w := getMedia(r, "333", fmt.Sprint(now.Unix()+2)); w.Code != 413 {
		t.Errorf("size cap: %d", w.Code)
	}
	if w := getMedia(r, "999", fmt.Sprint(now.Unix()+3)); w.Code != 404 {
		t.Errorf("unknown id: %d", w.Code)
	}
	if w := getMedia(r, "a..b", fmt.Sprint(now.Unix()+4)); w.Code != 400 {
		t.Errorf("bad id: %d", w.Code)
	}
	// unsigned / wrongly signed
	req := httptest.NewRequest("GET", "/media/111?ts="+ts(), nil)
	req.Header.Set("X-Notify-Signature", sign("GET /media/112?ts="+ts()))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("signature for another id: %d", w.Code)
	}
}

func TestMediaStreamCap(t *testing.T) {
	r, f, _ := setup(t)
	f.media = make([]byte, MaxMediaBytes+10) // file_size says small, the body is not
	if w := getMedia(r, "111", ts()); w.Code != 413 {
		t.Errorf("stream cap: %d", w.Code)
	}
}

func TestHelpers(t *testing.T) {
	if p, wa, ok := NormalizePhone("919431022239"); !ok || p != "9431022239" || wa != "919431022239" {
		t.Error("91 prefix")
	}
	if _, wa, ok := NormalizePhone("0000000029"); !ok || wa != "910000000029" {
		t.Error("dev test number refused")
	}
	if _, _, ok := NormalizePhone("19431022239"); ok {
		t.Error("11 digits accepted")
	}
	if AudioMime("audio/ogg; codecs=opus") != "audio/ogg" || AudioMime("text/html") != "" {
		t.Error("mime")
	}
	if !MediaURLAllowed("https://lookaside.fbsbx.com/whatsapp_business/attachments/?mid=1", "https://graph.facebook.com") ||
		MediaURLAllowed("http://lookaside.fbsbx.com/x", "https://graph.facebook.com") ||
		MediaURLAllowed("https://fbsbx.com.evil.io/x", "https://graph.facebook.com") ||
		MediaURLAllowed("https://u:p@lookaside.fbsbx.com/x", "https://graph.facebook.com") {
		t.Error("media url allowlist")
	}
	if !AudioURLAllowed("https://chat-api.mgdh.in/files/a.ogg", []string{"chat-api.mgdh.in"}) ||
		AudioURLAllowed("https://chat-api.mgdh.in:8443/a", []string{"chat-api.mgdh.in"}) ||
		AudioURLAllowed("file:///etc/passwd", []string{"chat-api.mgdh.in"}) ||
		AudioURLAllowed("https://x.chat-api.mgdh.in/a", []string{"chat-api.mgdh.in"}) {
		t.Error("audio url allowlist")
	}
	if config.PhoneNumberIDFromURL("https://graph.facebook.com/v21.0/1234567/messages") != "1234567" || config.PhoneNumberIDFromURL("http://127.0.0.1:9/no-send-on-dev") != "" {
		t.Error("pnid from url")
	}
}

func TestDocumentUploadThenSend(t *testing.T) {
	r, f, logs := setup(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("PK\x03\x04 fake xlsx"))
	body := `{"ts":` + ts() + `,"phone":"9431022239","document_base64":"` + b64 + `","mime":"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet","filename":"Sales Oct 2026.xlsx","caption":"Sales report 42.5 MT"}`
	w, out := post(r, body, "")
	if w.Code != 200 || out["wamid"] != "wamid.OUT1" {
		t.Fatalf("%d %v", w.Code, out)
	}
	if len(f.calls) != 2 || !strings.HasSuffix(f.calls[0], "/PNID/media") || !strings.HasSuffix(f.calls[1], "/PNID/messages") {
		t.Fatalf("calls %v", f.calls)
	}
	if !strings.Contains(f.bodies[0], `filename="Sales Oct 2026.xlsx"`) || !strings.Contains(f.bodies[0], "spreadsheetml.sheet") {
		t.Errorf("upload %s", f.bodies[0])
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(f.bodies[1]), &m)
	doc, _ := m["document"].(map[string]any)
	if m["type"] != "document" || doc["id"] != "MEDIA9" || doc["filename"] != "Sales Oct 2026.xlsx" || doc["caption"] != "Sales report 42.5 MT" {
		t.Errorf("payload %v", m)
	}
	if (*logs)[0].Template != "chat_document" {
		t.Errorf("log %+v", (*logs)[0])
	}
	if b, _ := json.Marshal((*logs)[0].Body); strings.Contains(string(b), "Sales") {
		t.Errorf("file name / caption stored in the log: %s", b)
	}
	// csv goes up as text/plain, the name keeps .csv
	csv := base64.StdEncoding.EncodeToString([]byte("a,b\n1,2\n"))
	w, _ = post(r, `{"ts":`+ts()+`,"phone":"9431022239","document_base64":"`+csv+`","mime":"text/csv","filename":"sales.csv"}`, "")
	if w.Code != 200 || !strings.Contains(f.bodies[2], "text/plain") || !strings.Contains(f.bodies[2], `filename="sales.csv"`) {
		t.Errorf("csv: %d %s", w.Code, f.bodies[2])
	}
}

func TestImageUploadThenSend(t *testing.T) {
	r, f, logs := setup(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("\x89PNG fake"))
	w, out := post(r, `{"ts":`+ts()+`,"phone":"9431022239","image_base64":"`+b64+`","mime":"image/png","caption":"Dispatch trend"}`, "")
	if w.Code != 200 || out["outcome"] != "sent" {
		t.Fatalf("%d %v", w.Code, out)
	}
	if !strings.Contains(f.bodies[0], `filename="image.png"`) || !strings.Contains(f.bodies[1], `"image":{"caption":"Dispatch trend","id":"MEDIA9"}`) {
		t.Errorf("bodies %v", f.bodies)
	}
	if (*logs)[0].Template != "chat_image" {
		t.Errorf("log %+v", (*logs)[0])
	}
	big := base64.StdEncoding.EncodeToString(make([]byte, MaxImageBytes+1))
	if w, _ := post(r, `{"ts":`+ts()+`,"phone":"9431022239","image_base64":"`+big+`","mime":"image/png"}`, ""); w.Code != 413 || len(f.calls) != 2 {
		t.Errorf("image cap: %d %v", w.Code, f.calls)
	}
}

func TestTemplateSend(t *testing.T) {
	r, f, logs := setup(t)
	body := `{"ts":` + ts() + `,"phone":"9431022239","template":{"name":"scheduled_report_ready","language":"hi","body_params":["Ramesh","Roz shaam sales report"],"button_payloads":["mbot:sched:7"]}}`
	w, out := post(r, body, "")
	if w.Code != 200 || out["wamid"] != "wamid.OUT1" {
		t.Fatalf("%d %v", w.Code, out)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(f.bodies[0]), &m)
	tpl, _ := m["template"].(map[string]any)
	if m["type"] != "template" || tpl["name"] != "scheduled_report_ready" || tpl["language"].(map[string]any)["code"] != "hi" {
		t.Fatalf("payload %v", m)
	}
	comps, _ := json.Marshal(tpl["components"])
	want := `[{"parameters":[{"text":"Ramesh","type":"text"},{"text":"Roz shaam sales report","type":"text"}],"type":"body"},{"index":"0","parameters":[{"payload":"mbot:sched:7","type":"payload"}],"sub_type":"quick_reply","type":"button"}]`
	if string(comps) != want {
		t.Errorf("components %s", comps)
	}
	if (*logs)[0].Template != "chat_template" {
		t.Errorf("log %+v", (*logs)[0])
	}
	if b, _ := json.Marshal((*logs)[0].Body); strings.Contains(string(b), "Ramesh") || !strings.Contains(string(b), "scheduled_report_ready") {
		t.Errorf("params stored / name missing in the log: %s", b)
	}
}

func TestMediaAndTemplateValidation(t *testing.T) {
	r, f, _ := setup(t)
	b := base64.StdEncoding.EncodeToString([]byte("x"))
	cases := map[string]string{
		"doc no filename":      `"phone":"9431022239","document_base64":"` + b + `","mime":"application/pdf"`,
		"doc path filename":    `"phone":"9431022239","document_base64":"` + b + `","mime":"application/pdf","filename":"../etc/passwd.pdf"`,
		"doc quote filename":   `"phone":"9431022239","document_base64":"` + b + `","mime":"application/pdf","filename":"a\"b.pdf"`,
		"doc bad mime":         `"phone":"9431022239","document_base64":"` + b + `","mime":"application/x-msdownload","filename":"a.exe"`,
		"image bad mime":       `"phone":"9431022239","image_base64":"` + b + `","mime":"image/gif"`,
		"caption on text":      `"phone":"9431022239","text":"hi","caption":"x"`,
		"filename on image":    `"phone":"9431022239","image_base64":"` + b + `","mime":"image/png","filename":"a.png"`,
		"long caption":         `"phone":"9431022239","image_base64":"` + b + `","mime":"image/png","caption":"` + strings.Repeat("a", CaptionLimit+1) + `"`,
		"doc and image":        `"phone":"9431022239","document_base64":"` + b + `","image_base64":"` + b + `","mime":"image/png"`,
		"template bad name":    `"phone":"9431022239","template":{"name":"Bad Name","language":"hi"}`,
		"template bad lang":    `"phone":"9431022239","template":{"name":"t1","language":"hindi"}`,
		"template empty param": `"phone":"9431022239","template":{"name":"t1","language":"hi","body_params":[" "]}`,
		"template newline":     `"phone":"9431022239","template":{"name":"t1","language":"hi","body_params":["a\nb"]}`,
		"template spaces":      `"phone":"9431022239","template":{"name":"t1","language":"hi","body_params":["a      b"]}`,
		"template reply_to":    `"phone":"9431022239","template":{"name":"t1","language":"hi"},"reply_to":"wamid.X"`,
		"template and text":    `"phone":"9431022239","template":{"name":"t1","language":"hi"},"text":"hi"`,
	}
	for name, c := range cases {
		w, out := post(r, `{"ts":`+ts()+`,`+c+`}`, "")
		if w.Code != 400 || out["outcome"] != "not_sent" {
			t.Errorf("%s: %d %v", name, w.Code, out)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("Meta called on invalid input: %v", f.calls)
	}
}
