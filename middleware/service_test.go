package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func sign(secret, msg string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestServiceSignatureOK(t *testing.T) {
	now := time.Unix(1_793_600_000, 0)
	uri := "/meta-templates/mahan_offer?refresh=1&ts=1793600000"
	if code, msg := ServiceSignatureOK("k", "GET", uri, sign("k", "GET "+uri), 1_793_600_000, now); code != 0 {
		t.Errorf("good signature refused: %d %s", code, msg)
	}
	if code, _ := ServiceSignatureOK("", "GET", uri, sign("", "GET "+uri), 1_793_600_000, now); code != 503 {
		t.Errorf("no secret must be 503, got %d", code)
	}
	if code, _ := ServiceSignatureOK("k", "GET", uri, sign("other", "GET "+uri), 1_793_600_000, now); code != 401 {
		t.Errorf("wrong key must be 401, got %d", code)
	}
	if code, _ := ServiceSignatureOK("k", "GET", uri, sign("k", "GET "+uri+"x"), 1_793_600_000, now); code != 401 {
		t.Errorf("signature over another uri must be 401, got %d", code)
	}
	if code, _ := ServiceSignatureOK("k", "GET", uri, sign("k", "GET "+uri), 1_793_600_000-400, now); code != 401 {
		t.Errorf("stale ts must be 401, got %d", code)
	}
	if code, _ := ServiceSignatureOK("k", "GET", uri, sign("k", "GET "+uri), 0, now); code != 401 {
		t.Errorf("missing ts must be 401, got %d", code)
	}
	if code, _ := ServiceSignatureOK("k", "GET", uri, "nope", 1_793_600_000, now); code != 401 {
		t.Errorf("malformed header must be 401, got %d", code)
	}
}
