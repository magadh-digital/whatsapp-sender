package metatemplates

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildPayloadOK(t *testing.T) {
	in := &CreateRequest{
		Name: "dispatch_alert", Category: "utility",
		Body:          "Dear {{1}}, your order {{2}} has left the plant. Thank you.",
		BodyExamples:  []string{"Ramesh", "SO-1042"},
		HeaderText:    "Order {{1}}",
		HeaderExample: "SO-1042",
		Footer:        "Magadh Industries",
		QuickReplies:  []string{"Thanks", "Call me"},
	}
	p, issues := BuildPayload(in)
	if len(issues) > 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
	b, _ := json.Marshal(p)
	s := string(b)
	for _, want := range []string{
		`"category":"UTILITY"`, `"language":"en"`,
		`"body_text":[["Ramesh","SO-1042"]]`, `"header_text":["SO-1042"]`,
		`{"text":"Thanks","type":"QUICK_REPLY"}`, `"type":"FOOTER"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("payload missing %s: %s", want, s)
		}
	}
}

func TestBuildPayloadRefusals(t *testing.T) {
	cases := map[string]CreateRequest{
		"Name":           {Name: "Bad Name", Category: "UTILITY", Body: "hello"},
		"Category":       {Name: "a", Category: "AUTHENTICATION", Body: "hello"},
		"without gaps":   {Name: "a", Category: "UTILITY", Body: "Hi {{1}} and {{3}} ok", BodyExamples: []string{"x", "y"}},
		"example value":  {Name: "a", Category: "UTILITY", Body: "Hi {{1}} ok"},
		"first or last":  {Name: "a", Category: "UTILITY", Body: "{{1}} is here", BodyExamples: []string{"x"}},
		"no names":       {Name: "a", Category: "UTILITY", Body: "Hi {{name}} ok"},
		"Footer":         {Name: "a", Category: "UTILITY", Body: "hello", Footer: "Hi {{1}}"},
		"25 characters":  {Name: "a", Category: "MARKETING", Body: "hello", QuickReplies: []string{strings.Repeat("x", 26)}},
		"Body: required": {Name: "a", Category: "UTILITY"},
	}
	for want, in := range cases {
		in := in
		_, issues := BuildPayload(&in)
		if !strings.Contains(strings.Join(issues, "|"), want) {
			t.Errorf("%q: issues %v do not mention it", want, issues)
		}
	}
}
