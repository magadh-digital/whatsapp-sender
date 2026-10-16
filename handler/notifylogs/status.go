package notifylogs

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"notify/config"
	"notify/db"
	"notify/handler/metatemplates"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Meta's message statuses, as its webhook sends them (entry[].changes[].value.statuses[]), forwarded by mds-orders.
type MetaStatus struct {
	ID          string `json:"id"` // the wamid models.SendMessage stored in payload.messages[0].id
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"` // unix seconds, as a string
	RecipientID string `json:"recipient_id"`
	Errors      []struct {
		Code      int    `json:"code"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		ErrorData struct {
			Details string `json:"details"`
		} `json:"error_data"`
	} `json:"errors"`
}

// statusRank: a later webhook never moves a row backwards (read stays read when a late "delivered" arrives).
var statusRank = map[string]int{"sent": 1, "delivered": 2, "read": 3, "failed": 4}

// SignatureOK checks X-Notify-Signature: sha256=<hex HMAC of the exact body with the shared secret>.
func SignatureOK(secret string, body []byte, header string) bool {
	if secret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// StatusUpdates builds the two idempotent updates for one status: the per-status time ($min keeps the first one
// when Meta repeats itself) and the current status, which only moves forward (the filter's rank condition).
// ok=false for a status this service does not track.
func StatusUpdates(st MetaStatus, now time.Time) (stamp bson.M, advanceFilter bson.M, advance bson.M, ok bool) {
	rank, known := statusRank[st.Status]
	if !known || st.ID == "" {
		return nil, nil, nil, false
	}
	at := now
	if sec, err := strconv.ParseInt(st.Timestamp, 10, 64); err == nil && sec > 0 {
		at = time.Unix(sec, 0)
	}
	stamp = bson.M{"$min": bson.M{st.Status + "_at": at}}

	set := bson.M{"delivery_status": st.Status, "delivery_rank": rank, "status_updated_at": now}
	if st.Status == "failed" && len(st.Errors) > 0 {
		e := st.Errors[0]
		set["error_code"] = e.Code
		set["error_title"] = e.Title
		detail := e.ErrorData.Details
		if detail == "" {
			detail = e.Message
		}
		set["error_detail"] = detail
	}
	advanceFilter = bson.M{
		"payload.messages.id": st.ID,
		"$or":                 bson.A{bson.M{"delivery_rank": bson.M{"$exists": false}}, bson.M{"delivery_rank": bson.M{"$lt": rank}}},
	}
	return stamp, advanceFilter, bson.M{"$set": set}, true
}

// Status: POST /meta-status {statuses:[…]} — from mds-orders' Meta webhook, HMAC-signed with NOTIFY_STATUS_SECRET.
// Unknown wamids (messages this service did not send) are counted and ignored. Always cheap; never calls Meta.
func Status(ctx *gin.Context) {
	secret := config.GetMetaConfig().StatusSecret
	if secret == "" {
		ctx.JSON(503, gin.H{"message": "Status intake off: set NOTIFY_STATUS_SECRET"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 1<<20))
	if err != nil {
		ctx.JSON(400, gin.H{"message": "Unreadable body"})
		return
	}
	if !SignatureOK(secret, raw, ctx.GetHeader("X-Notify-Signature")) {
		ctx.JSON(401, gin.H{"message": "Bad signature"})
		return
	}
	var in struct {
		Statuses []MetaStatus `json:"statuses"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		ctx.JSON(400, gin.H{"message": "Unreadable JSON: " + err.Error()})
		return
	}
	if db.MessageLogModel == nil {
		ctx.JSON(503, gin.H{"message": "Message log store not connected"})
		return
	}
	ensureIndexes()

	c := context.Background()
	now := time.Now()
	matched, advanced, unknown, skipped := 0, 0, 0, 0
	for _, st := range in.Statuses {
		stamp, f, adv, ok := StatusUpdates(st, now)
		if !ok {
			skipped++
			continue
		}
		res, err := db.MessageLogModel.UpdateOne(c, bson.M{"payload.messages.id": st.ID}, stamp)
		if err != nil {
			log.Println("[meta-status]", st.ID, err)
			continue
		}
		if res.MatchedCount == 0 {
			unknown++
			continue
		}
		matched++
		if r2, err := db.MessageLogModel.UpdateOne(c, f, adv); err == nil && r2.ModifiedCount > 0 {
			advanced++
		}
	}
	ctx.JSON(200, gin.H{"matched": matched, "advanced": advanced, "unknown": unknown, "skipped": skipped})
}

// One: GET /notify-logs/:wamid — one message by its Meta id, with the times of every status received.
// (Meta has no API to ask for a status; this is what its webhook has told us.)
func One(ctx *gin.Context) {
	wamid := ctx.Param("wamid")
	if wamid == "" || len(wamid) > 200 {
		ctx.JSON(400, gin.H{"message": "Message id required"})
		return
	}
	if db.MessageLogModel == nil {
		ctx.JSON(503, gin.H{"message": "Message log store not connected"})
		return
	}
	var d logDoc
	err := db.MessageLogModel.FindOne(context.Background(), bson.M{"payload.messages.id": wamid}, options.FindOne()).Decode(&d)
	if err == mongo.ErrNoDocuments {
		ctx.JSON(404, gin.H{"message": "No message with id " + wamid + " in this service's log"})
		return
	}
	if err != nil {
		ctx.JSON(500, gin.H{"message": "Could not read the log: " + err.Error()})
		return
	}
	cats := metatemplates.CachedCategories()
	r := ToRow(d, IsOtpTemplate(d.Template, cats[d.Template]))
	history := []gin.H{}
	for _, h := range []struct {
		s string
		t *time.Time
	}{{"sent", d.SentAt}, {"delivered", d.DeliveredAt}, {"read", d.ReadAt}, {"failed", d.FailedAt}} {
		if h.t != nil {
			history = append(history, gin.H{"status": h.s, "at": h.t})
		}
	}
	sort.SliceStable(history, func(i, j int) bool { return history[i]["at"].(*time.Time).Before(*history[j]["at"].(*time.Time)) })
	ctx.JSON(200, gin.H{"result": r, "history": history})
}
