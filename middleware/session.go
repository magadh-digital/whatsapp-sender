package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"notify/config"

	"github.com/gin-gonic/gin"
)

// SessionUser is the part of mds-orders' validate-session answer the template routes need.
type SessionUser struct {
	ID              string
	Name            string
	UserType        string
	IsAdminEmployee bool
	Permissions     map[string]bool
}

func (u *SessionUser) Can(permission string) bool {
	return u.UserType == "super_admin" || u.Permissions[permission]
}

const sessionUserKey = "session_user"

type cachedSession struct {
	user    *SessionUser
	expires time.Time
}

var (
	sessionCache sync.Map // sha256(app-session-id) -> cachedSession
	authClient   = &http.Client{Timeout: 8 * time.Second}
)

const sessionTTL = 60 * time.Second

type validateAnswer struct {
	User struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		UserType string `json:"user_type"`
	} `json:"user"`
	IsAdminEmployee bool `json:"isAdminEmployee"`
	Permissions     []struct {
		Permission string `json:"permission"`
	} `json:"permissions"`
	Message string `json:"message"`
}

// validateSession asks mds-orders who owns this console session. Only successful answers are cached.
func validateSession(sid string) (*SessionUser, int, string) {
	sum := sha256.Sum256([]byte(sid))
	key := hex.EncodeToString(sum[:])
	if v, ok := sessionCache.Load(key); ok {
		c := v.(cachedSession)
		if time.Now().Before(c.expires) {
			return c.user, 200, ""
		}
		sessionCache.Delete(key)
	}

	url := config.GetMetaConfig().AuthValidateURL
	if url == "" {
		return nil, 503, "Caller sign-in check not configured: set AUTH_VALIDATE_URL (mds-orders …/api/v1/auth/validate-session/server)"
	}

	body, _ := json.Marshal(map[string]string{"app_session_id": sid})
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 503, "Sign-in check failed: " + err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := authClient.Do(req)
	if err != nil {
		return nil, 503, "Sign-in service unreachable: " + err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var ans validateAnswer
	_ = json.Unmarshal(raw, &ans)
	if resp.StatusCode != 200 || ans.User.ID == "" {
		msg := "Session expired or not valid, sign in again"
		if ans.Message != "" {
			msg += " (" + ans.Message + ")"
		}
		return nil, 401, msg
	}

	user := &SessionUser{
		ID:              ans.User.ID,
		Name:            ans.User.Name,
		UserType:        ans.User.UserType,
		IsAdminEmployee: ans.IsAdminEmployee,
		Permissions:     make(map[string]bool, len(ans.Permissions)),
	}
	for _, p := range ans.Permissions {
		user.Permissions[p.Permission] = true
	}
	sessionCache.Store(key, cachedSession{user: user, expires: time.Now().Add(sessionTTL)})
	return user, 200, ""
}

// RequireStaff lets through a console session of a super admin, admin or employee (mds-orders' isAdminEmployee).
func RequireStaff() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		sid := ctx.GetHeader("app-session-id")
		if sid == "" {
			ctx.AbortWithStatusJSON(401, gin.H{"message": "Sign in required (app-session-id header missing)"})
			return
		}
		user, status, msg := validateSession(sid)
		if user == nil {
			ctx.AbortWithStatusJSON(status, gin.H{"message": msg})
			return
		}
		if !user.IsAdminEmployee {
			ctx.AbortWithStatusJSON(403, gin.H{"message": "Staff only"})
			return
		}
		ctx.Set(sessionUserKey, user)
		ctx.Next()
	}
}

// RequirePermission must run after RequireStaff.
func RequirePermission(permission string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		user := GetSessionUser(ctx)
		if user == nil || !user.Can(permission) {
			ctx.AbortWithStatusJSON(403, gin.H{"message": "Missing permission " + permission})
			return
		}
		ctx.Next()
	}
}

func GetSessionUser(ctx *gin.Context) *SessionUser {
	v, ok := ctx.Get(sessionUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*SessionUser)
	return u
}
