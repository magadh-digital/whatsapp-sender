package notifylogs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func sign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestSignatureOK(t *testing.T) {
	body := `{"statuses":[]}`
	if !SignatureOK("s3cret", []byte(body), sign("s3cret", body)) {
		t.Error("good signature refused")
	}
	if SignatureOK("s3cret", []byte(body+" "), sign("s3cret", body)) || SignatureOK("s3cret", []byte(body), sign("other", body)) {
		t.Error("bad signature accepted")
	}
	if SignatureOK("", []byte(body), sign("", body)) {
		t.Error("empty secret must refuse")
	}
}

func TestStatusUpdates(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	stamp, f, adv, ok := StatusUpdates(MetaStatus{ID: "wamid.A", Status: "delivered", Timestamp: "1792130000"}, now)
	if !ok {
		t.Fatal("delivered not tracked")
	}
	if at := stamp["$min"].(bson.M)["delivered_at"].(time.Time); at.Unix() != 1792130000 {
		t.Errorf("timestamp %v", at)
	}
	// only advances rows below rank 2: a "read" row (3) is not moved back
	cond := f["$or"].(bson.A)[1].(bson.M)["delivery_rank"].(bson.M)["$lt"]
	if cond != 2 || adv["$set"].(bson.M)["delivery_status"] != "delivered" {
		t.Errorf("advance %v %v", f, adv)
	}

	st := MetaStatus{ID: "wamid.B", Status: "failed", Timestamp: "x"}
	st.Errors = append(st.Errors, struct {
		Code      int    `json:"code"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		ErrorData struct {
			Details string `json:"details"`
		} `json:"error_data"`
	}{Code: 131026, Title: "Message undeliverable", Message: "Message undeliverable"})
	stamp, _, adv, ok = StatusUpdates(st, now)
	set := adv["$set"].(bson.M)
	if !ok || set["error_code"] != 131026 || set["error_detail"] != "Message undeliverable" || !stamp["$min"].(bson.M)["failed_at"].(time.Time).Equal(now) {
		t.Errorf("failed: %v %v", stamp, set)
	}
	if _, _, _, ok := StatusUpdates(MetaStatus{ID: "wamid.C", Status: "deleted"}, now); ok {
		t.Error("unknown status tracked")
	}
	if _, _, _, ok := StatusUpdates(MetaStatus{Status: "read"}, now); ok {
		t.Error("status without id tracked")
	}
}
