package templates

// POST /templates/send-message: queue one template message to each phone.
//
//	{ "id": "<template name>", "phone": ["9876543210", …],
//	  "language": "en",                       optional: which language version (default: the only one, else en)
//	  "params": ["v1", "v2"],                 positional body values {{1}}..{{n}}   ─┐ Meta's definition
//	  "variables": {"1": "v1", "2": "v2"},    the same, keyed "1".."n"              ─┤ (no preset needed, 2026-11-02)
//	  "header": "...", "buttons": {"2": "x"}, header value / link, URL-button values ─┘
//	  "variables": {"date": "…"},             NAMED values: the send preset of that name maps them (the ERP's callers)
//	  "ref": "<batch id>" }                   kept on every log row, /notify-logs?ref= lists the batch
//
// Named variables need a preset (as always). Positional values, or no values at all, are resolved from Meta's own
// template list: the template must be APPROVED there. When Meta cannot be consulted and a preset exists, a send
// without values still goes through the preset, so the zero-variable notices keep working.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"notify/handler/metatemplates"
	"notify/models"
)

type SendRequest struct {
	ID        string            `json:"id" binding:"required"`
	Language  string            `json:"language"`
	Variables map[string]string `json:"variables"`
	Params    []string          `json:"params"`
	Header    string            `json:"header"`
	Buttons   map[string]string `json:"buttons"`
	Phone     []string          `json:"phone" binding:"required"`
	Ref       string            `json:"ref"`
}

type PresetLookup func(name string) (*models.WhatsappTemplate, error)
type MetaLookup func(name, language string) (*metatemplates.Row, string)

// splitVariables: the numeric keys "1".."n" in order (positional), and whether any non-numeric key (named) exists.
func splitVariables(v map[string]string) (positional []string, named bool) {
	idx := []int{}
	byIdx := map[int]string{}
	for k, val := range v {
		n, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil || n < 1 {
			named = true
			continue
		}
		idx = append(idx, n)
		byIdx[n] = val
	}
	sort.Ints(idx)
	for i, n := range idx {
		if n != i+1 {
			// a gap ("1", "3"): keep what is there in order, Meta's count check reports the mismatch
			break
		}
		positional = append(positional, byIdx[n])
	}
	return positional, named
}

func statusFor(msg string) int {
	switch {
	case strings.Contains(msg, "not configured"), strings.HasPrefix(msg, "Could not reach"), strings.HasPrefix(msg, "Meta refused the template list"):
		return 503
	case strings.HasPrefix(msg, "No template named"), strings.Contains(msg, "has no "):
		return 404
	case strings.Contains(msg, "not APPROVED"):
		return 409
	}
	return 400
}

// Plan decides how the request is sent and builds the message to queue; a refusal comes back as (nil, status, text).
func Plan(req SendRequest, preset PresetLookup, meta MetaLookup) (*models.WhatsappTemplate, int, string) {
	positional, named := splitVariables(req.Variables)
	if named {
		p, err := preset(req.ID)
		if err != nil {
			return nil, 404, fmt.Sprintf("No send preset named %s for named variables; send positional values (params) instead", req.ID)
		}
		if missing := models.ValidateVariables(p, req.Variables); len(missing) > 0 {
			return nil, 400, strings.Join(missing, "\n")
		}
		return models.GenerateWhatsappMessage(p, req.Variables), 0, ""
	}

	body := req.Params
	if len(body) == 0 {
		body = positional
	}
	row, msg := meta(req.ID, strings.TrimSpace(req.Language))
	if row == nil {
		// Meta unavailable (or the template unknown there) and nothing to fill: the preset still describes a plain send
		if len(body) == 0 && req.Header == "" && len(req.Buttons) == 0 && statusFor(msg) != 409 {
			if p, err := preset(req.ID); err == nil && len(p.ExtractVariables()) == 0 {
				return p, 0, ""
			}
		}
		return nil, statusFor(msg), msg
	}
	buttons := map[int]string{}
	for k, v := range req.Buttons {
		if n, err := strconv.Atoi(strings.TrimSpace(k)); err == nil {
			buttons[n] = v
		}
	}
	t, issues := metatemplates.BuildSend(row, metatemplates.SendValues{Body: body, Header: req.Header, Buttons: buttons})
	if len(issues) > 0 {
		return nil, 400, strings.Join(issues, "\n")
	}
	return t, 0, ""
}
