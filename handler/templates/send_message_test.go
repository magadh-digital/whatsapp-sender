package templates

import (
	"errors"
	"strings"
	"testing"

	"notify/handler/metatemplates"
	"notify/models"
)

func presetWith(name string, keys ...string) *models.WhatsappTemplate {
	p := &models.WhatsappTemplate{Name: name}
	p.Language.Code = "en"
	p.Components = []models.TemplateComponent{}
	if len(keys) > 0 {
		params := []models.TemplateParameter{}
		for _, k := range keys {
			params = append(params, models.TemplateParameter{Type: "text", Text: k})
		}
		p.Components = append(p.Components, models.TemplateComponent{Type: "body", Parameters: params})
	}
	return p
}

func TestSplitVariables(t *testing.T) {
	pos, named := splitVariables(map[string]string{"2": "b", "1": "a"})
	if named || len(pos) != 2 || pos[0] != "a" || pos[1] != "b" {
		t.Errorf("positional: %v %v", pos, named)
	}
	if _, named := splitVariables(map[string]string{"date": "x"}); !named {
		t.Error("named not detected")
	}
	if pos, named := splitVariables(nil); named || len(pos) != 0 {
		t.Error("empty")
	}
}

func TestPlan(t *testing.T) {
	// a fresh document per lookup, as Mongo gives one (the fill replaces parameter keys in place)
	presets := map[string]func() *models.WhatsappTemplate{
		"sale_close": func() *models.WhatsappTemplate { return presetWith("sale_close", "date") },
		"notice":     func() *models.WhatsappTemplate { return presetWith("notice") },
	}
	preset := func(name string) (*models.WhatsappTemplate, error) {
		if mk, ok := presets[name]; ok {
			return mk(), nil
		}
		return nil, errors.New("mongo: no documents in result")
	}
	rows := []metatemplates.Row{
		{Name: "notice", Language: "en", Status: "APPROVED", Body: "Plain notice"},
		{Name: "greet", Language: "en", Status: "APPROVED", Body: "Hi {{1}}, {{2}} MT", Variables: 2},
		{Name: "waiting", Language: "en", Status: "PENDING", Body: "Soon"},
	}
	meta := func(name, lang string) (*metatemplates.Row, string) { return metatemplates.PickRow(rows, name, lang) }
	metaDown := func(string, string) (*metatemplates.Row, string) { return nil, "Could not reach Meta: timeout" }

	// named variables: the preset path, values replace the keys
	m, st, msg := Plan(SendRequest{ID: "sale_close", Variables: map[string]string{"date": "02-11-2026"}, Phone: []string{"1"}}, preset, meta)
	if msg != "" || st != 0 || m.Components[0].Parameters[0].Text != "02-11-2026" {
		t.Errorf("named: %+v %d %q", m, st, msg)
	}
	if _, st, msg := Plan(SendRequest{ID: "sale_close", Variables: map[string]string{"date": ""}}, preset, meta); st != 400 || !strings.Contains(msg, "date is empty") {
		t.Errorf("named missing: %d %q", st, msg)
	}
	if _, st, msg := Plan(SendRequest{ID: "greet", Variables: map[string]string{"name": "x"}}, preset, meta); st != 404 || !strings.Contains(msg, "No send preset named greet") {
		t.Errorf("named without preset: %d %q", st, msg)
	}

	// positional values: Meta's definition, no preset involved
	m, st, msg = Plan(SendRequest{ID: "greet", Params: []string{"Ramesh", "12"}}, preset, meta)
	if msg != "" || len(m.Components) != 1 || m.Components[0].Parameters[1].Text != "12" || m.Language.Code != "en" {
		t.Errorf("positional params: %+v %d %q", m, st, msg)
	}
	m, _, msg = Plan(SendRequest{ID: "greet", Variables: map[string]string{"2": "12", "1": "Ramesh"}}, preset, meta)
	if msg != "" || m.Components[0].Parameters[0].Text != "Ramesh" {
		t.Errorf("positional keyed variables: %+v %q", m, msg)
	}
	if _, st, msg := Plan(SendRequest{ID: "greet", Params: []string{"only one"}}, preset, meta); st != 400 || !strings.Contains(msg, "1 value(s) given, the template has 2") {
		t.Errorf("count mismatch: %d %q", st, msg)
	}

	// no values: Meta's definition, a template without variables sends with no components
	m, _, msg = Plan(SendRequest{ID: "notice"}, preset, meta)
	if msg != "" || len(m.Components) != 0 {
		t.Errorf("plain: %+v %q", m, msg)
	}
	if _, st, msg := Plan(SendRequest{ID: "waiting"}, preset, meta); st != 409 || !strings.Contains(msg, "PENDING") {
		t.Errorf("not approved: %d %q", st, msg)
	}
	if _, st, msg := Plan(SendRequest{ID: "ghost"}, preset, meta); st != 404 || !strings.Contains(msg, "No template named ghost") {
		t.Errorf("unknown: %d %q", st, msg)
	}

	// Meta unreachable: a zero-variable preset still carries a plain send; anything else is a 503
	m, st, msg = Plan(SendRequest{ID: "notice"}, preset, metaDown)
	if msg != "" || st != 0 || m.Name != "notice" {
		t.Errorf("fallback to preset: %+v %d %q", m, st, msg)
	}
	if _, st, msg := Plan(SendRequest{ID: "greet", Params: []string{"a", "b"}}, preset, metaDown); st != 503 || !strings.Contains(msg, "Could not reach Meta") {
		t.Errorf("meta down with values: %d %q", st, msg)
	}
}
