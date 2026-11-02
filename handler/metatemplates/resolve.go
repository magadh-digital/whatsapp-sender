package metatemplates

// Sending from Meta's own definition (2026-11-02): /templates/send-message no longer needs a send preset for a
// template. `Resolve` finds the APPROVED template in the cached Meta list and `BuildSend` turns the caller's values
// into Meta's components using what the template actually has: {{n}} body variables, a text / image / video /
// document header, URL buttons with a variable. Presets stay for the callers that still send NAMED variables.

import (
	"fmt"
	"strings"

	"notify/config"
	"notify/models"
)

// Resolve: the APPROVED Meta template `name`. language "" = the template's only language, or its "en" version when
// it exists in several. The refusal text says what is wrong (not configured, not on Meta, which language, not approved).
func Resolve(name, language string) (*Row, string) {
	cfg := config.GetMetaConfig()
	if msg := notConfigured(cfg); msg != "" {
		return nil, msg
	}
	rows, _, _, merr, err := loadRows(cfg, false)
	if err != nil {
		return nil, "Could not reach Meta: " + err.Error()
	}
	if merr != nil {
		return nil, "Meta refused the template list: " + merr.Text()
	}
	return PickRow(rows, name, language)
}

// Lookup is Resolve without the APPROVED requirement: the template in that language whatever its status (a
// campaign waiting for approval polls it); refresh bypasses the 60 s cache.
func Lookup(name, language string, refresh bool) (*Row, string) {
	cfg := config.GetMetaConfig()
	if msg := notConfigured(cfg); msg != "" {
		return nil, msg
	}
	rows, _, _, merr, err := loadRows(cfg, refresh)
	if err != nil {
		return nil, "Could not reach Meta: " + err.Error()
	}
	if merr != nil {
		return nil, "Meta refused the template list: " + merr.Text()
	}
	return PickRowAny(rows, name, language)
}

// PickRow is Resolve's pure part over an already fetched list: the APPROVED template in that language.
func PickRow(rows []Row, name, language string) (*Row, string) {
	pick, msg := PickRowAny(rows, name, language)
	if pick == nil {
		return nil, msg
	}
	if pick.Status != "APPROVED" {
		return nil, fmt.Sprintf("%s (%s) is %s on Meta, not APPROVED: Meta will not deliver it", pick.Name, pick.Language, pick.Status)
	}
	return pick, ""
}

// PickRowAny: the template in that language whatever its status ("" = its only language, or "en" among several).
func PickRowAny(rows []Row, name, language string) (*Row, string) {
	matches := []Row{}
	for _, r := range rows {
		if r.Name == name {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Sprintf("No template named %s on Meta", name)
	}
	langs := make([]string, 0, len(matches))
	for _, m := range matches {
		langs = append(langs, m.Language)
	}
	var pick *Row
	switch {
	case language != "":
		for i := range matches {
			if matches[i].Language == language {
				pick = &matches[i]
			}
		}
		if pick == nil {
			return nil, fmt.Sprintf("%s has no %s version on Meta (it has: %s)", name, language, strings.Join(langs, ", "))
		}
	case len(matches) == 1:
		pick = &matches[0]
	default:
		for i := range matches {
			if matches[i].Language == "en" {
				pick = &matches[i]
			}
		}
		if pick == nil {
			return nil, fmt.Sprintf("%s exists in several languages on Meta (%s): say which with \"language\"", name, strings.Join(langs, ", "))
		}
	}
	return pick, ""
}

// SendValues is what a send supplies for a Meta template.
type SendValues struct {
	// the body's {{1}}..{{n}} in order
	Body []string
	// a text header's {{1}}, or the public link of an image / video / document header
	Header string
	// a URL button's {{1}} by the button's position (1-based) among the template's buttons
	Buttons map[int]string
}

// BuildSend: Meta's components for one send of `row`, from Meta's own definition. The issues name what is missing
// or does not fit; a template without variables gives an empty component list.
func BuildSend(row *Row, v SendValues) (*models.WhatsappTemplate, []string) {
	issues := []string{}
	t := &models.WhatsappTemplate{Name: row.Name}
	t.Language.Code = row.Language
	t.Components = []models.TemplateComponent{}

	header := strings.TrimSpace(v.Header)
	switch strings.ToUpper(row.HeaderFormat) {
	case "", "TEXT":
		if len(distinctVars(row.HeaderText)) > 0 {
			if header == "" {
				issues = append(issues, "header: the header has a {{1}}, give its value")
			} else {
				t.Components = append(t.Components, models.TemplateComponent{Type: "header", Parameters: []models.TemplateParameter{{Type: "text", Text: header}}})
			}
		} else if header != "" {
			issues = append(issues, "header: this template's header has no variable")
		}
	case "IMAGE":
		if header == "" {
			issues = append(issues, "header: the template has an image header, give the image link")
		} else {
			t.Components = append(t.Components, models.TemplateComponent{Type: "header", Parameters: []models.TemplateParameter{{Type: "image", Image: &models.MediaLink{Link: header}}}})
		}
	case "VIDEO":
		if header == "" {
			issues = append(issues, "header: the template has a video header, give the video link")
		} else {
			t.Components = append(t.Components, models.TemplateComponent{Type: "header", Parameters: []models.TemplateParameter{{Type: "video", Video: &models.MediaRef{Link: header}}}})
		}
	case "DOCUMENT":
		if header == "" {
			issues = append(issues, "header: the template has a document header, give the document link")
		} else {
			t.Components = append(t.Components, models.TemplateComponent{Type: "header", Parameters: []models.TemplateParameter{{Type: "document", Document: &models.DocumentRef{Link: header}}}})
		}
	default:
		issues = append(issues, "header: "+strings.ToUpper(row.HeaderFormat)+" headers are not supported here")
	}

	if row.Variables > 0 || len(v.Body) > 0 {
		if len(v.Body) != row.Variables {
			issues = append(issues, fmt.Sprintf("body: %d value(s) given, the template has %d variable(s)", len(v.Body), row.Variables))
		} else {
			params := make([]models.TemplateParameter, 0, len(v.Body))
			for i, s := range v.Body {
				if strings.TrimSpace(s) == "" {
					issues = append(issues, fmt.Sprintf("body: value %d ({{%d}}) is empty", i+1, i+1))
				}
				params = append(params, models.TemplateParameter{Type: "text", Text: s})
			}
			t.Components = append(t.Components, models.TemplateComponent{Type: "body", Parameters: params})
		}
	}

	for i, b := range row.Buttons {
		if b.Type != "URL" || len(distinctVars(b.URL)) == 0 {
			continue
		}
		val := strings.TrimSpace(v.Buttons[i+1])
		if val == "" {
			issues = append(issues, fmt.Sprintf("button %d: its URL has a {{1}}, give its value", i+1))
			continue
		}
		t.Components = append(t.Components, models.TemplateComponent{Type: "button", SubType: "url", Index: uint(i), Parameters: []models.TemplateParameter{{Type: "text", Text: val}}})
	}

	if len(issues) > 0 {
		return nil, issues
	}
	return t, nil
}
