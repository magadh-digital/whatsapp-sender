package metatemplates

import (
	"strings"
	"testing"
)

func TestPickRow(t *testing.T) {
	rows := []Row{
		{Name: "plain", Language: "en", Status: "APPROVED"},
		{Name: "two", Language: "en", Status: "APPROVED"},
		{Name: "two", Language: "hi", Status: "APPROVED"},
		{Name: "pending", Language: "en", Status: "PENDING"},
		{Name: "hionly", Language: "hi", Status: "APPROVED"},
		{Name: "pair", Language: "hi", Status: "APPROVED"},
		{Name: "pair", Language: "gu", Status: "APPROVED"},
	}
	if r, msg := PickRow(rows, "plain", ""); msg != "" || r.Language != "en" {
		t.Errorf("plain: %v %q", r, msg)
	}
	if r, msg := PickRow(rows, "hionly", ""); msg != "" || r.Language != "hi" {
		t.Errorf("single language picked without asking: %v %q", r, msg)
	}
	if r, msg := PickRow(rows, "two", ""); msg != "" || r.Language != "en" {
		t.Errorf("several languages default to en: %v %q", r, msg)
	}
	if r, msg := PickRow(rows, "two", "hi"); msg != "" || r.Language != "hi" {
		t.Errorf("asked language: %v %q", r, msg)
	}
	if _, msg := PickRow(rows, "two", "gu"); !strings.Contains(msg, "no gu version") || !strings.Contains(msg, "en, hi") {
		t.Errorf("missing language: %q", msg)
	}
	if _, msg := PickRow(rows, "pair", ""); !strings.Contains(msg, "several languages") {
		t.Errorf("ambiguous without en must ask: %q", msg)
	}
	if _, msg := PickRow(rows, "nope", ""); !strings.Contains(msg, "No template named nope") {
		t.Errorf("unknown: %q", msg)
	}
	if _, msg := PickRow(rows, "pending", ""); !strings.Contains(msg, "PENDING") || !strings.Contains(msg, "not APPROVED") {
		t.Errorf("not approved: %q", msg)
	}
}

func TestBuildSend(t *testing.T) {
	plain := &Row{Name: "notice", Language: "en", Status: "APPROVED", Body: "Hello all"}
	m, issues := BuildSend(plain, SendValues{})
	if len(issues) > 0 || len(m.Components) != 0 || m.Name != "notice" || m.Language.Code != "en" {
		t.Fatalf("plain: %v %+v", issues, m)
	}
	if _, issues := BuildSend(plain, SendValues{Body: []string{"x"}}); len(issues) == 0 || !strings.Contains(issues[0], "1 value(s) given, the template has 0") {
		t.Errorf("extra body value must be refused: %v", issues)
	}

	rich := &Row{
		Name: "order_update", Language: "en", Status: "APPROVED",
		HeaderFormat: "TEXT", HeaderText: "Order {{1}}",
		Body: "Dear {{1}}, your order {{2}} has left.", Variables: 2,
		Buttons: []metaButton{{Type: "QUICK_REPLY", Text: "Thanks"}, {Type: "URL", Text: "Track", URL: "https://mgdh.in/t/{{1}}"}},
	}
	m, issues = BuildSend(rich, SendValues{Body: []string{"Ramesh", "SO-1"}, Header: "SO-1", Buttons: map[int]string{2: "SO-1"}})
	if len(issues) > 0 {
		t.Fatalf("rich: %v", issues)
	}
	if len(m.Components) != 3 || m.Components[0].Type != "header" || m.Components[0].Parameters[0].Text != "SO-1" {
		t.Errorf("header component: %+v", m.Components)
	}
	if m.Components[1].Type != "body" || len(m.Components[1].Parameters) != 2 || m.Components[1].Parameters[1].Text != "SO-1" {
		t.Errorf("body component: %+v", m.Components[1])
	}
	if b := m.Components[2]; b.Type != "button" || b.SubType != "url" || b.Index != 1 || b.Parameters[0].Text != "SO-1" {
		t.Errorf("button component: %+v", b)
	}
	_, issues = BuildSend(rich, SendValues{Body: []string{"Ramesh", ""}})
	joined := strings.Join(issues, "|")
	for _, want := range []string{"header: the header has a {{1}}", "body: value 2 ({{2}}) is empty", "button 2: its URL has a {{1}}"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues %v lack %q", issues, want)
		}
	}

	img := &Row{Name: "promo", Language: "en", Status: "APPROVED", HeaderFormat: "IMAGE", Body: "See {{1}}", Variables: 1}
	if _, issues := BuildSend(img, SendValues{Body: []string{"this"}}); len(issues) != 1 || !strings.Contains(issues[0], "image link") {
		t.Errorf("image header without link: %v", issues)
	}
	m, issues = BuildSend(img, SendValues{Body: []string{"this"}, Header: "https://x/y.jpg"})
	if len(issues) > 0 || m.Components[0].Parameters[0].Type != "image" || m.Components[0].Parameters[0].Image.Link != "https://x/y.jpg" {
		t.Errorf("image header: %v %+v", issues, m.Components)
	}
	loc := &Row{Name: "map", Language: "en", Status: "APPROVED", HeaderFormat: "LOCATION", Body: "Here"}
	if _, issues := BuildSend(loc, SendValues{}); len(issues) != 1 || !strings.Contains(issues[0], "LOCATION headers are not supported") {
		t.Errorf("location header: %v", issues)
	}
}
