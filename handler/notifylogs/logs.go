// Package notifylogs reads what this service sent: one Mongo `message_logs` row per WhatsApp send, written by
// models.SendMessage with Meta's answer (SUCCESS = Meta accepted it, with its wamid; FAILED = Meta's error).
// Delivery receipts (sent / delivered / read / failed) come later from Meta's webhook, which points at mds-orders;
// mds-orders forwards the statuses to POST /meta-status (status.go), which stamps them on the row by wamid. Meta has
// no API to ask for a message's status, so the webhook is the only source.
//
// For the console's WhatsApp Templates > Sent log (staff only). Phones are shown in full (owner, 16 Oct 2026); the
// variables of OTP / authentication templates are replaced by "••••".
package notifylogs

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"notify/db"
	"notify/handler/metatemplates"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const Masked = "••••"

var (
	ist       = time.FixedZone("IST", 5*3600+1800)
	nonDigit  = regexp.MustCompile(`\D`)
	otpNameRe = regexp.MustCompile(`(?i)otp|verif|passcode|login_code|auth`)
	dayRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	indexOnce sync.Once
	maxDays   = 31
	// the Sent log's status filter
	statusFilters = []string{"sent", "delivered", "read", "failed"}
)

// IsOtpTemplate: Meta category AUTHENTICATION, or a name that says OTP / verification.
func IsOtpTemplate(name, category string) bool {
	return category == "AUTHENTICATION" || otpNameRe.MatchString(name)
}

// DayRange turns from/to (YYYY-MM-DD, IST, inclusive) into [start, end). Empty = today. Bad input = an error text.
func DayRange(from, to string, now time.Time) (time.Time, time.Time, string) {
	today := now.In(ist).Format("2006-01-02")
	if from == "" {
		from = today
	}
	if to == "" {
		to = from
	}
	if !dayRe.MatchString(from) || !dayRe.MatchString(to) {
		return time.Time{}, time.Time{}, "Dates as YYYY-MM-DD"
	}
	start, err1 := time.ParseInLocation("2006-01-02", from, ist)
	end, err2 := time.ParseInLocation("2006-01-02", to, ist)
	if err1 != nil || err2 != nil {
		return time.Time{}, time.Time{}, "Dates as YYYY-MM-DD"
	}
	end = end.AddDate(0, 0, 1)
	if !end.After(start) {
		return time.Time{}, time.Time{}, "From must be on or before To"
	}
	if end.Sub(start) > time.Duration(maxDays)*24*time.Hour {
		return time.Time{}, time.Time{}, "At most 31 days at a time"
	}
	return start, end, ""
}

type logDoc struct {
	ID        primitive.ObjectID `bson:"_id"`
	Template  string             `bson:"template"`
	Phone     string             `bson:"phone"`
	Status    string             `bson:"status"`
	CreatedAt time.Time          `bson:"created_at"`
	Body      bson.Raw           `bson:"body"`
	Payload   bson.Raw           `bson:"payload"`
	// written by POST /meta-status from Meta's webhook
	Delivery    string     `bson:"delivery_status"`
	SentAt      *time.Time `bson:"sent_at"`
	DeliveredAt *time.Time `bson:"delivered_at"`
	ReadAt      *time.Time `bson:"read_at"`
	FailedAt    *time.Time `bson:"failed_at"`
	ErrorCode   int        `bson:"error_code"`
	ErrorTitle  string     `bson:"error_title"`
	ErrorDetail string     `bson:"error_detail"`
}

// Row is one send as the console shows it.
type Row struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Template  string    `json:"template"`
	To        string    `json:"to"`
	Variables []string  `json:"variables"`
	// sent | delivered | read | failed: the furthest the message got (failed = Meta refused the send or the webhook said so)
	Status string `json:"status"`
	// Meta accepted the send call (it then has a message id); false = refused on the spot, see Error
	Accepted bool `json:"accepted"`
	// the last status Meta's webhook reported, "" before the first one arrives
	Delivery    string         `json:"delivery,omitempty"`
	SentAt      *time.Time     `json:"sent_at,omitempty"`
	DeliveredAt *time.Time     `json:"delivered_at,omitempty"`
	ReadAt      *time.Time     `json:"read_at,omitempty"`
	FailedAt    *time.Time     `json:"failed_at,omitempty"`
	ErrorCode   int            `json:"error_code,omitempty"`
	MessageID   string         `json:"message_id,omitempty"`
	Error       string         `json:"error,omitempty"`
	OtpMasked   bool           `json:"otp_masked"`
	Body        map[string]any `json:"body"`
	Response    map[string]any `json:"response"`
}

func rawToMap(r bson.Raw) map[string]any {
	out := map[string]any{}
	if len(r) == 0 {
		return out
	}
	ext, err := bson.MarshalExtJSON(r, false, false)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(ext, &out)
	return out
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
func asStr(v any) string {
	s, _ := v.(string)
	return s
}

// ToRow maps one stored send; otp masks the variables (in the row and in the body copy).
func ToRow(d logDoc, otp bool) Row {
	body := rawToMap(d.Body)
	resp := rawToMap(d.Payload)
	r := Row{
		ID: d.ID.Hex(), Time: d.CreatedAt, Template: d.Template, To: d.Phone,
		Variables: []string{}, OtpMasked: otp, Body: body, Response: resp, Status: "failed",
		Accepted: d.Status == "SUCCESS", Delivery: d.Delivery,
		SentAt: d.SentAt, DeliveredAt: d.DeliveredAt, ReadAt: d.ReadAt, FailedAt: d.FailedAt, ErrorCode: d.ErrorCode,
	}
	if r.Accepted {
		r.Status = "sent"
		if d.Delivery == "delivered" || d.Delivery == "read" || d.Delivery == "failed" {
			r.Status = d.Delivery
		}
	}
	for _, c := range asList(body["components"]) {
		for _, p := range asList(asMap(c)["parameters"]) {
			pm := asMap(p)
			if pm == nil {
				continue
			}
			switch asStr(pm["type"]) {
			case "text":
				if otp {
					pm["text"] = Masked
				}
				r.Variables = append(r.Variables, asStr(pm["text"]))
			case "image", "video", "document":
				r.Variables = append(r.Variables, "["+asStr(pm["type"])+"]")
			}
		}
	}
	if msgs := asList(resp["messages"]); len(msgs) > 0 {
		r.MessageID = asStr(asMap(msgs[0])["id"])
	}
	if e := asMap(resp["error"]); e != nil {
		r.Error = asStr(e["message"])
		if det := asStr(asMap(e["error_data"])["details"]); det != "" && det != r.Error {
			r.Error += ": " + det
		}
	}
	if d.Delivery == "failed" {
		r.Error = strings.TrimSpace(strings.Trim(d.ErrorTitle+": "+d.ErrorDetail, ": "))
		if r.Error == "" {
			r.Error = "Meta reported the message as failed"
		}
	}
	return r
}

// BuildFilter: the Mongo query for the date range and the optional template / phone (last 10 digits) / status.
func BuildFilter(start, end time.Time, template, phone, status string) bson.M {
	q := bson.M{"created_at": bson.M{"$gte": start, "$lt": end}}
	if template != "" {
		q["template"] = template
	}
	if d := nonDigit.ReplaceAllString(phone, ""); d != "" {
		if len(d) > 10 {
			d = d[len(d)-10:]
		}
		q["phone"] = bson.M{"$regex": d + "$"}
	}
	switch status {
	case "sent": // accepted, nothing further yet
		q["status"] = "SUCCESS"
		q["delivery_status"] = bson.M{"$nin": bson.A{"delivered", "read", "failed"}}
	case "delivered", "read":
		q["status"] = "SUCCESS"
		q["delivery_status"] = status
	case "failed":
		q["$or"] = bson.A{bson.M{"status": "FAILED"}, bson.M{"delivery_status": "failed"}}
	}
	return q
}

func validStatus(s string) bool {
	for _, x := range statusFilters {
		if x == s {
			return true
		}
	}
	return false
}

func ensureIndexes() {
	indexOnce.Do(func() {
		for _, keys := range []bson.D{{{Key: "created_at", Value: -1}}, {{Key: "payload.messages.id", Value: 1}}} {
			if _, err := db.MessageLogModel.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: keys}); err != nil {
				log.Println("[notify-logs] index:", err)
			}
		}
	})
}

// List: GET /notify-logs?from=&to=&template=&phone=&status=sent|delivered|read|failed&page=&limit=
func List(ctx *gin.Context) {
	if db.MessageLogModel == nil {
		ctx.JSON(503, gin.H{"message": "Message log store not connected"})
		return
	}
	ensureIndexes()

	start, end, bad := DayRange(ctx.Query("from"), ctx.Query("to"), time.Now())
	if bad != "" {
		ctx.JSON(400, gin.H{"message": bad})
		return
	}
	status := ctx.Query("status")
	if status != "" && !validStatus(status) {
		ctx.JSON(400, gin.H{"message": "Status: sent, delivered, read or failed"})
		return
	}
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	template := strings.TrimSpace(ctx.Query("template"))
	phone := ctx.Query("phone")
	filter := BuildFilter(start, end, template, phone, status)
	c := context.Background()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetSkip(int64((page - 1) * limit)).SetLimit(int64(limit))
	cur, err := db.MessageLogModel.Find(c, filter, opts)
	if err != nil {
		ctx.JSON(500, gin.H{"message": "Could not read the log: " + err.Error()})
		return
	}
	defer cur.Close(c)

	categories := metatemplates.CachedCategories()
	rows := make([]Row, 0, limit)
	for cur.Next(c) {
		var d logDoc
		if err := cur.Decode(&d); err != nil {
			continue
		}
		rows = append(rows, ToRow(d, IsOtpTemplate(d.Template, categories[d.Template])))
	}
	total, _ := db.MessageLogModel.CountDocuments(c, filter)

	// counts per status under the same filters except status (for the console's pills)
	counts := gin.H{}
	for _, word := range statusFilters {
		n, _ := db.MessageLogModel.CountDocuments(c, BuildFilter(start, end, template, phone, word))
		counts[word] = n
	}

	ctx.JSON(200, gin.H{
		"result": rows, "total": total, "page": page, "limit": limit, "counts": counts,
		"from": start.Format("2006-01-02"), "to": end.AddDate(0, 0, -1).Format("2006-01-02"),
	})
}
