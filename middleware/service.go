package middleware

// Service calls without a console session: a GET signed with the shared service key NOTIFY_STATUS_SECRET, the
// scheme the chat routes already use (X-Notify-Signature: sha256=<hex HMAC-SHA256 of "GET <request URI>">, with a
// `ts` query (unix seconds) within 5 minutes). mds-orders' scheduled check of a composed broadcast's template polls
// GET /meta-templates/:name this way (2026-11-02).

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"notify/config"

	"github.com/gin-gonic/gin"
)

const serviceCallerKey = "service_caller"

// MaxServiceSkew: how old a signed service request may be.
const MaxServiceSkew = 5 * time.Minute

// ServiceSignatureOK: 0 when the signed request is accepted, else the HTTP status and message to answer with.
func ServiceSignatureOK(secret, method, requestURI, header string, ts int64, now time.Time) (int, string) {
	if secret == "" {
		return 503, "Service calls off: set NOTIFY_STATUS_SECRET"
	}
	if ts <= 0 {
		return 401, "Stale or missing ts"
	}
	if d := now.Sub(time.Unix(ts, 0)); d > MaxServiceSkew || d < -MaxServiceSkew {
		return 401, "Stale or missing ts"
	}
	if !strings.HasPrefix(header, "sha256=") {
		return 401, "Bad signature"
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return 401, "Bad signature"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(method + " " + requestURI))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return 401, "Bad signature"
	}
	return 0, ""
}

// RequireStaffOrService lets through a staff console session (RequireStaff) or a service-signed request.
func RequireStaffOrService() gin.HandlerFunc {
	staff := RequireStaff()
	return func(ctx *gin.Context) {
		sig := ctx.GetHeader("X-Notify-Signature")
		if sig != "" && ctx.GetHeader("app-session-id") == "" {
			ts, _ := strconv.ParseInt(ctx.Query("ts"), 10, 64)
			code, msg := ServiceSignatureOK(config.GetMetaConfig().StatusSecret, ctx.Request.Method, ctx.Request.URL.RequestURI(), sig, ts, time.Now())
			if code != 0 {
				ctx.AbortWithStatusJSON(code, gin.H{"message": msg})
				return
			}
			ctx.Set(serviceCallerKey, true)
			ctx.Next()
			return
		}
		staff(ctx)
	}
}

// IsServiceCaller: the request was accepted by its service signature (no session user).
func IsServiceCaller(ctx *gin.Context) bool {
	v, ok := ctx.Get(serviceCallerKey)
	return ok && v == true
}
