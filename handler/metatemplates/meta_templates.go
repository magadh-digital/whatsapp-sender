// Package metatemplates lists and creates the company's WhatsApp message templates on Meta (the WhatsApp Business
// Account), for the console's Master > WhatsApp Templates page.
//
// Not to be confused with /templates: those are this service's send presets (Mongo), which map a Meta template
// name to the parameter shape /templates/send-message fills. A template created here becomes sendable through
// /templates/send-message only once Meta approves it AND a send preset with the same name exists ("wired").
package metatemplates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"notify/config"
	"notify/middleware"
	"notify/models"

	"github.com/gin-gonic/gin"
)

const listFields = "id,name,status,category,language,components,rejected_reason,quality_score"

var graphClient = &http.Client{Timeout: 20 * time.Second}

// ---------------------------------------------------------------- shapes

type metaButton struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	URL         string `json:"url,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

type metaComponent struct {
	Type    string          `json:"type"`
	Format  string          `json:"format,omitempty"`
	Text    string          `json:"text,omitempty"`
	Buttons []metaButton    `json:"buttons,omitempty"`
	Example json.RawMessage `json:"example,omitempty"`
}

type metaTemplate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Category       string `json:"category"`
	Language       string `json:"language"`
	RejectedReason string `json:"rejected_reason"`
	QualityScore   *struct {
		Score string `json:"score"`
	} `json:"quality_score"`
	Components []metaComponent `json:"components"`
}

// Row is one template as the console shows it.
type Row struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Language       string       `json:"language"`
	Category       string       `json:"category"`
	Status         string       `json:"status"`
	RejectedReason string       `json:"rejected_reason,omitempty"`
	QualityScore   string       `json:"quality_score,omitempty"`
	HeaderFormat   string       `json:"header_format,omitempty"`
	HeaderText     string       `json:"header_text,omitempty"`
	Body           string       `json:"body"`
	BodyExamples   []string     `json:"body_examples"`
	Variables      int          `json:"variables"`
	Footer         string       `json:"footer,omitempty"`
	Buttons        []metaButton `json:"buttons"`
	// a send preset with this name exists here, so /templates/send-message can send it
	Wired bool `json:"wired"`
}

type metaError struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Code      int    `json:"code"`
	Subcode   int    `json:"error_subcode"`
	UserTitle string `json:"error_user_title"`
	UserMsg   string `json:"error_user_msg"`
	FbtraceID string `json:"fbtrace_id"`
}

func (e *metaError) Text() string {
	switch {
	case e.UserMsg != "" && e.UserTitle != "":
		return e.UserTitle + ": " + e.UserMsg
	case e.UserMsg != "":
		return e.UserMsg
	case e.Message != "":
		return e.Message
	}
	return "Meta refused the request"
}

// ---------------------------------------------------------------- Graph calls

func notConfigured(cfg config.MetaConfig) string {
	missing := make([]string, 0, 2)
	if cfg.WabaID == "" {
		missing = append(missing, "WHATSAPP_WABA_ID (the WhatsApp Business Account id)")
	}
	if cfg.Token == "" {
		missing = append(missing, "WHATSAPP_MANAGE_TOKEN or WHATSAPP_AUTH (a token with whatsapp_business_management)")
	}
	if len(missing) == 0 {
		return ""
	}
	return "WhatsApp Business account not configured on the notify service: set " + strings.Join(missing, " and ")
}

// graph does one call; a non-2xx answer comes back as *metaError with the HTTP status.
func graph(cfg config.MetaConfig, method, fullURL string, body any) ([]byte, int, *metaError, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, fullURL, rdr)
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := graphClient.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var wrap struct {
			Error metaError `json:"error"`
		}
		_ = json.Unmarshal(raw, &wrap)
		if wrap.Error.Message == "" {
			wrap.Error.Message = fmt.Sprintf("Meta answered HTTP %d", resp.StatusCode)
		}
		return raw, resp.StatusCode, &wrap.Error, nil
	}
	return raw, resp.StatusCode, nil, nil
}

func templatesURL(cfg config.MetaConfig) string {
	return fmt.Sprintf("%s/%s/%s/message_templates", cfg.GraphBaseURL, cfg.GraphVersion, url.PathEscape(cfg.WabaID))
}

func fetchAll(cfg config.MetaConfig) ([]metaTemplate, *metaError, error) {
	next := templatesURL(cfg) + "?limit=100&fields=" + url.QueryEscape(listFields)
	out := make([]metaTemplate, 0)
	for page := 0; next != "" && page < 20; page++ {
		raw, _, merr, err := graph(cfg, "GET", next, nil)
		if err != nil || merr != nil {
			return nil, merr, err
		}
		var res struct {
			Data   []metaTemplate `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, nil, fmt.Errorf("unreadable answer from Meta: %w", err)
		}
		out = append(out, res.Data...)
		next = res.Paging.Next
	}
	return out, nil, nil
}

func toRow(t metaTemplate, wired map[string]bool) Row {
	r := Row{
		ID: t.ID, Name: t.Name, Language: t.Language, Category: t.Category, Status: t.Status,
		BodyExamples: []string{}, Buttons: []metaButton{}, Wired: wired[t.Name],
	}
	if t.RejectedReason != "" && t.RejectedReason != "NONE" {
		r.RejectedReason = t.RejectedReason
	}
	if t.QualityScore != nil && t.QualityScore.Score != "UNKNOWN" {
		r.QualityScore = t.QualityScore.Score
	}
	for _, c := range t.Components {
		switch c.Type {
		case "HEADER":
			r.HeaderFormat = c.Format
			r.HeaderText = c.Text
		case "BODY":
			r.Body = c.Text
			var ex struct {
				BodyText [][]string `json:"body_text"`
			}
			if len(c.Example) > 0 && json.Unmarshal(c.Example, &ex) == nil && len(ex.BodyText) > 0 {
				r.BodyExamples = ex.BodyText[0]
			}
		case "FOOTER":
			r.Footer = c.Text
		case "BUTTONS":
			r.Buttons = append(r.Buttons, c.Buttons...)
		}
	}
	r.Variables = len(distinctVars(r.Body))
	return r
}

// ---------------------------------------------------------------- cache (60 s)

var cache struct {
	sync.Mutex
	rows    []Row
	fetched time.Time
}

const cacheTTL = 60 * time.Second

func wiredNames() map[string]bool {
	names := map[string]bool{}
	list, err := models.ListTemplates()
	if err != nil {
		return names
	}
	for _, t := range list {
		names[t.Name] = true
	}
	return names
}

func loadRows(cfg config.MetaConfig, refresh bool) ([]Row, time.Time, bool, *metaError, error) {
	cache.Lock()
	defer cache.Unlock()
	if !refresh && cache.rows != nil && time.Since(cache.fetched) < cacheTTL {
		return cache.rows, cache.fetched, true, nil, nil
	}
	list, merr, err := fetchAll(cfg)
	if err != nil || merr != nil {
		return nil, time.Time{}, false, merr, err
	}
	wired := wiredNames()
	rows := make([]Row, 0, len(list))
	for _, t := range list {
		rows = append(rows, toRow(t, wired))
	}
	cache.rows = rows
	cache.fetched = time.Now()
	return rows, cache.fetched, false, nil, nil
}

func dropCache() {
	cache.Lock()
	cache.rows = nil
	cache.Unlock()
}

func metaFailure(ctx *gin.Context, status int, merr *metaError, err error) {
	if err != nil {
		ctx.JSON(502, gin.H{"message": "Could not reach Meta: " + err.Error()})
		return
	}
	ctx.JSON(status, gin.H{"message": merr.Text(), "meta_error": merr})
}

// ---------------------------------------------------------------- handlers

// List: GET /meta-templates[?refresh=1]
func List(ctx *gin.Context) {
	cfg := config.GetMetaConfig()
	if msg := notConfigured(cfg); msg != "" {
		ctx.JSON(503, gin.H{"message": msg})
		return
	}
	rows, fetched, cached, merr, err := loadRows(cfg, ctx.Query("refresh") == "1")
	if err != nil || merr != nil {
		metaFailure(ctx, 502, merr, err)
		return
	}
	ctx.JSON(200, gin.H{"result": rows, "fetched_at": fetched, "cached": cached})
}

// Get: GET /meta-templates/:name — every language of one template.
func Get(ctx *gin.Context) {
	cfg := config.GetMetaConfig()
	if msg := notConfigured(cfg); msg != "" {
		ctx.JSON(503, gin.H{"message": msg})
		return
	}
	rows, _, _, merr, err := loadRows(cfg, ctx.Query("refresh") == "1")
	if err != nil || merr != nil {
		metaFailure(ctx, 502, merr, err)
		return
	}
	name := ctx.Param("name")
	out := make([]Row, 0)
	for _, r := range rows {
		if r.Name == name {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		ctx.JSON(404, gin.H{"message": "No template named " + name})
		return
	}
	ctx.JSON(200, gin.H{"result": out})
}

// CreateRequest is what the console posts.
type CreateRequest struct {
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Language      string   `json:"language"`
	Body          string   `json:"body"`
	BodyExamples  []string `json:"body_examples"`
	HeaderText    string   `json:"header_text"`
	HeaderExample string   `json:"header_example"`
	Footer        string   `json:"footer"`
	QuickReplies  []string `json:"quick_replies"`
}

// Create: POST /meta-templates[?dry_run=1] — validates, then submits to Meta for approval.
// dry_run answers the exact payload Meta would receive without sending it.
func Create(ctx *gin.Context) {
	var in CreateRequest
	if err := ctx.ShouldBindJSON(&in); err != nil {
		ctx.JSON(400, gin.H{"message": "Unreadable request: " + err.Error()})
		return
	}
	payload, issues := BuildPayload(&in)
	if len(issues) > 0 {
		ctx.JSON(400, gin.H{"message": strings.Join(issues, "\n"), "issues": issues})
		return
	}

	cfg := config.GetMetaConfig()
	if ctx.Query("dry_run") == "1" {
		ctx.JSON(200, gin.H{"dry_run": true, "configured": notConfigured(cfg) == "", "payload": payload})
		return
	}
	if msg := notConfigured(cfg); msg != "" {
		ctx.JSON(503, gin.H{"message": msg})
		return
	}

	// a friendlier refusal than Meta's when the name + language already exists
	if rows, _, _, merr, err := loadRows(cfg, false); err == nil && merr == nil {
		for _, r := range rows {
			if r.Name == in.Name && r.Language == in.Language {
				ctx.JSON(409, gin.H{"message": fmt.Sprintf("A template %s (%s) already exists with status %s", r.Name, r.Language, r.Status)})
				return
			}
		}
	}

	user := middleware.GetSessionUser(ctx)
	raw, status, merr, err := graph(cfg, "POST", templatesURL(cfg), payload)
	if err != nil || merr != nil {
		if merr != nil && status >= 400 && status < 500 {
			status = 400
		} else {
			status = 502
		}
		if user != nil {
			log.Printf("[meta-templates] create %s by %s (%s) refused: %v %v", in.Name, user.Name, user.ID, err, merr)
		}
		metaFailure(ctx, status, merr, err)
		return
	}
	dropCache()
	var res map[string]any
	_ = json.Unmarshal(raw, &res)
	if user != nil {
		log.Printf("[meta-templates] created %s (%s, %s) by %s (%s): %s", in.Name, in.Language, in.Category, user.Name, user.ID, string(raw))
	}
	ctx.JSON(200, gin.H{"message": "Submitted to Meta for approval", "result": res})
}

// ---------------------------------------------------------------- validation

var (
	nameRe     = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)
	langRe     = regexp.MustCompile(`^[a-z]{2,3}(_[A-Z]{2})?$`)
	varRe      = regexp.MustCompile(`\{\{\s*(\d+)\s*\}\}`)
	anyBraceRe = regexp.MustCompile(`\{\{[^}]*\}\}`)
)

func distinctVars(text string) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, m := range varRe.FindAllStringSubmatch(text, -1) {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// checkVars: numbered {{1}}…{{n}} with no gaps, no named or malformed placeholders. Returns n.
func checkVars(label, text string, issues *[]string) int {
	if len(anyBraceRe.FindAllString(text, -1)) != len(varRe.FindAllString(text, -1)) {
		*issues = append(*issues, label+": use numbered variables like {{1}}, {{2}} (no names inside the braces)")
	}
	vars := distinctVars(text)
	for i := 1; i <= len(vars); i++ {
		found := false
		for _, v := range vars {
			if v == i {
				found = true
			}
		}
		if !found {
			*issues = append(*issues, fmt.Sprintf("%s: variables must be numbered {{1}} to {{%d}} without gaps", label, len(vars)))
			break
		}
	}
	return len(vars)
}

// BuildPayload validates the request and builds Meta's message_templates body.
func BuildPayload(in *CreateRequest) (map[string]any, []string) {
	issues := []string{}
	in.Name = strings.TrimSpace(in.Name)
	in.Category = strings.ToUpper(strings.TrimSpace(in.Category))
	in.Language = strings.TrimSpace(in.Language)
	if in.Language == "" {
		in.Language = "en"
	}
	in.Body = strings.TrimSpace(in.Body)
	in.HeaderText = strings.TrimSpace(in.HeaderText)
	in.Footer = strings.TrimSpace(in.Footer)

	if !nameRe.MatchString(in.Name) {
		issues = append(issues, "Name: lowercase letters, digits and _ only (e.g. dispatch_alert)")
	}
	switch in.Category {
	case "UTILITY", "MARKETING":
	case "AUTHENTICATION":
		issues = append(issues, "Category: AUTHENTICATION (OTP) templates have a fixed Meta format; create them in WhatsApp Manager")
	default:
		issues = append(issues, "Category: UTILITY or MARKETING")
	}
	if !langRe.MatchString(in.Language) {
		issues = append(issues, "Language: a Meta language code such as en, en_US or hi")
	}

	components := []map[string]any{}

	if in.HeaderText != "" {
		if len([]rune(in.HeaderText)) > 60 {
			issues = append(issues, "Header: at most 60 characters")
		}
		if strings.ContainsAny(in.HeaderText, "\n\r") {
			issues = append(issues, "Header: one line only")
		}
		n := checkVars("Header", in.HeaderText, &issues)
		h := map[string]any{"type": "HEADER", "format": "TEXT", "text": in.HeaderText}
		if n > 1 {
			issues = append(issues, "Header: at most one variable, {{1}}")
		} else if n == 1 {
			ex := strings.TrimSpace(in.HeaderExample)
			if ex == "" {
				issues = append(issues, "Header: give an example value for {{1}}")
			}
			h["example"] = map[string]any{"header_text": []string{ex}}
		}
		components = append(components, h)
	}

	if in.Body == "" {
		issues = append(issues, "Body: required")
	} else {
		if len([]rune(in.Body)) > 1024 {
			issues = append(issues, "Body: at most 1024 characters")
		}
		n := checkVars("Body", in.Body, &issues)
		if strings.HasPrefix(in.Body, "{{") || strings.HasSuffix(in.Body, "}}") {
			issues = append(issues, "Body: a variable can't be the very first or last thing; put words around it")
		}
		b := map[string]any{"type": "BODY", "text": in.Body}
		if n > 0 {
			ex := make([]string, n)
			for i := 0; i < n; i++ {
				if i < len(in.BodyExamples) {
					ex[i] = strings.TrimSpace(in.BodyExamples[i])
				}
				if ex[i] == "" {
					issues = append(issues, fmt.Sprintf("Body: give an example value for {{%d}}", i+1))
				}
			}
			b["example"] = map[string]any{"body_text": [][]string{ex}}
		}
		components = append(components, b)
	}

	if in.Footer != "" {
		if len([]rune(in.Footer)) > 60 {
			issues = append(issues, "Footer: at most 60 characters")
		}
		if anyBraceRe.MatchString(in.Footer) {
			issues = append(issues, "Footer: no variables")
		}
		components = append(components, map[string]any{"type": "FOOTER", "text": in.Footer})
	}

	if len(in.QuickReplies) > 0 {
		if len(in.QuickReplies) > 10 {
			issues = append(issues, "Buttons: at most 10 quick replies")
		}
		seen := map[string]bool{}
		buttons := []map[string]any{}
		for i, q := range in.QuickReplies {
			q = strings.TrimSpace(q)
			if q == "" {
				issues = append(issues, fmt.Sprintf("Button %d: empty", i+1))
				continue
			}
			if len([]rune(q)) > 25 {
				issues = append(issues, fmt.Sprintf("Button %d: at most 25 characters", i+1))
			}
			if seen[strings.ToLower(q)] {
				issues = append(issues, fmt.Sprintf("Button %d: duplicate text", i+1))
			}
			seen[strings.ToLower(q)] = true
			buttons = append(buttons, map[string]any{"type": "QUICK_REPLY", "text": q})
		}
		components = append(components, map[string]any{"type": "BUTTONS", "buttons": buttons})
	}

	return map[string]any{
		"name":       in.Name,
		"language":   in.Language,
		"category":   in.Category,
		"components": components,
	}, issues
}
