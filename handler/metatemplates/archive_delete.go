package metatemplates

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"

	"notify/config"
	"notify/middleware"
	"notify/redis"

	"github.com/gin-gonic/gin"
)

// Archive is local to this service: a Redis set of template names the console hides by default. Meta is not told.

var hsmIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func archivedNames() map[string]bool {
	out := map[string]bool{}
	if redis.RedisClient == nil {
		return out
	}
	names, err := redis.RedisClient.SMembers(context.Background(), redis.RedisKeys.MetaTemplateArchived).Result()
	if err != nil {
		log.Println("[meta-templates] archived set unreadable:", err)
		return out
	}
	for _, n := range names {
		out[n] = true
	}
	return out
}

// applyArchived marks rows (copies; the cached slice is never touched) and drops archived ones unless asked for.
func applyArchived(rows []Row, archived map[string]bool, showArchived bool) ([]Row, int) {
	out := make([]Row, 0, len(rows))
	hidden := 0
	for _, r := range rows {
		r.Archived = archived[r.Name]
		if r.Archived && !showArchived {
			hidden++
			continue
		}
		out = append(out, r)
	}
	return out, hidden
}

func deleteURL(cfg config.MetaConfig, name, hsmID string) string {
	q := url.Values{}
	q.Set("name", name)
	if hsmID != "" {
		q.Set("hsm_id", hsmID)
	}
	return templatesURL(cfg) + "?" + q.Encode()
}

// Delete: DELETE /meta-templates/:name[?hsm_id=] — deletes on Meta (all languages, or the one hsm_id).
func Delete(ctx *gin.Context) {
	name := ctx.Param("name")
	hsmID := ctx.Query("hsm_id")
	if !nameRe.MatchString(name) {
		ctx.JSON(400, gin.H{"message": "Not a template name: " + name})
		return
	}
	if hsmID != "" && !hsmIDRe.MatchString(hsmID) {
		ctx.JSON(400, gin.H{"message": "Not a template id: " + hsmID})
		return
	}
	cfg := config.GetMetaConfig()
	if msg := notConfigured(cfg); msg != "" {
		ctx.JSON(503, gin.H{"message": msg})
		return
	}
	user := middleware.GetSessionUser(ctx)
	raw, status, merr, err := graph(cfg, "DELETE", deleteURL(cfg, name, hsmID), nil)
	if err != nil || merr != nil {
		if merr != nil && status >= 400 && status < 500 {
			status = 400
		} else {
			status = 502
		}
		metaFailure(ctx, status, merr, err)
		return
	}
	dropCache()
	if hsmID == "" && redis.RedisClient != nil {
		redis.RedisClient.SRem(context.Background(), redis.RedisKeys.MetaTemplateArchived, name)
	}
	var res map[string]any
	_ = json.Unmarshal(raw, &res)
	if user != nil {
		log.Printf("[meta-templates] deleted %s hsm_id=%q by %s (%s): %s", name, hsmID, user.Name, user.ID, string(raw))
	}
	ctx.JSON(200, gin.H{"message": "Deleted on Meta", "result": res})
}

func setArchived(ctx *gin.Context, on bool) {
	name := ctx.Param("name")
	if !nameRe.MatchString(name) {
		ctx.JSON(400, gin.H{"message": "Not a template name: " + name})
		return
	}
	if redis.RedisClient == nil {
		ctx.JSON(503, gin.H{"message": "Redis not connected"})
		return
	}
	var err error
	if on {
		err = redis.RedisClient.SAdd(context.Background(), redis.RedisKeys.MetaTemplateArchived, name).Err()
	} else {
		err = redis.RedisClient.SRem(context.Background(), redis.RedisKeys.MetaTemplateArchived, name).Err()
	}
	if err != nil {
		ctx.JSON(500, gin.H{"message": "Could not save: " + err.Error()})
		return
	}
	word := "Unarchived"
	if on {
		word = "Archived"
	}
	if user := middleware.GetSessionUser(ctx); user != nil {
		log.Printf("[meta-templates] %s %s by %s (%s)", word, name, user.Name, user.ID)
	}
	ctx.JSON(200, gin.H{"message": fmt.Sprintf("%s %s", word, name), "archived": on})
}

// Archive: POST /meta-templates/:name/archive
func Archive(ctx *gin.Context) { setArchived(ctx, true) }

// Unarchive: POST /meta-templates/:name/unarchive
func Unarchive(ctx *gin.Context) { setArchived(ctx, false) }

// CachedCategories: template name -> Meta category from the last list fetch (no Meta call); empty before one.
func CachedCategories() map[string]string {
	cache.Lock()
	defer cache.Unlock()
	out := make(map[string]string, len(cache.rows))
	for _, r := range cache.rows {
		out[r.Name] = r.Category
	}
	return out
}
