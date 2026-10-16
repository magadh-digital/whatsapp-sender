package notifylogs

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestMaskPhone(t *testing.T) {
	cases := map[string]string{"9431022243": "94•••••243", "919431022243": "91•••••••243", "123": "•••"}
	for in, want := range cases {
		if got := MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestIsOtpTemplate(t *testing.T) {
	if !IsOtpTemplate("login_otp", "") || !IsOtpTemplate("anything", "AUTHENTICATION") || !IsOtpTemplate("verify_number", "UTILITY") {
		t.Error("OTP templates not recognised")
	}
	if IsOtpTemplate("sale_close", "UTILITY") {
		t.Error("sale_close is not an OTP template")
	}
}

func TestDayRange(t *testing.T) {
	now := time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC) // 01:30 IST on the 17th
	s, e, bad := DayRange("", "", now)
	if bad != "" || s.Format(time.RFC3339) != "2026-10-17T00:00:00+05:30" || e.Sub(s) != 24*time.Hour {
		t.Errorf("today in IST: %v %v %q", s, e, bad)
	}
	if _, _, bad := DayRange("2026-10-10", "2026-10-01", now); bad == "" {
		t.Error("from after to accepted")
	}
	if _, _, bad := DayRange("2026-08-01", "2026-10-01", now); bad == "" {
		t.Error("62 days accepted")
	}
	if _, _, bad := DayRange("16/10/2026", "", now); bad == "" {
		t.Error("bad format accepted")
	}
}

func TestBuildFilter(t *testing.T) {
	s, e, _ := DayRange("2026-10-16", "", time.Now())
	q := BuildFilter(s, e, "sale_close", "+91 94310-22243", "failed")
	if q["template"] != "sale_close" || q["status"] != "FAILED" {
		t.Errorf("filter %v", q)
	}
	if re := q["phone"].(bson.M)["$regex"]; re != "9431022243$" {
		t.Errorf("phone regex %v", re)
	}
	if _, ok := BuildFilter(s, e, "", "", "nope")["status"]; ok {
		t.Error("unknown status filtered")
	}
}

func doc(t *testing.T, tpl, status string, body, payload bson.M) logDoc {
	b, _ := bson.Marshal(body)
	p, _ := bson.Marshal(payload)
	return logDoc{ID: primitive.NewObjectID(), Template: tpl, Phone: "9431022243", Status: status, CreatedAt: time.Now(), Body: b, Payload: p}
}

func TestToRow(t *testing.T) {
	body := bson.M{"components": bson.A{bson.M{"type": "body", "parameters": bson.A{bson.M{"type": "text", "text": "4821"}, bson.M{"type": "text", "text": "10 min"}}}}}
	ok := bson.M{"contacts": bson.A{bson.M{"input": "919431022243", "wa_id": "919431022243"}}, "messages": bson.A{bson.M{"id": "wamid.X"}}}
	r := ToRow(doc(t, "login_otp", "SUCCESS", body, ok), true)
	if r.Status != "sent" || r.MessageID != "wamid.X" || r.To != "94•••••243" {
		t.Errorf("row %+v", r)
	}
	if strings.Join(r.Variables, ",") != Masked+","+Masked {
		t.Errorf("OTP variables not masked: %v", r.Variables)
	}
	if s := r.Response["contacts"].([]any)[0].(map[string]any)["wa_id"]; s != "91•••••••243" {
		t.Errorf("contact not masked: %v", s)
	}
	if strings.Contains(strings.Join(r.Variables, ""), "4821") {
		t.Error("OTP leaked")
	}

	failed := bson.M{"error": bson.M{"message": "(#132001) Template name does not exist in the translation", "error_data": bson.M{"details": "template name (x) does not exist in en"}}}
	r = ToRow(doc(t, "sale_close", "FAILED", body, failed), false)
	if r.Status != "failed" || !strings.Contains(r.Error, "does not exist in en") || r.Variables[0] != "4821" {
		t.Errorf("failed row %+v", r)
	}
}
