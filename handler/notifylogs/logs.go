// Package notifylogs reads what this service sent: one Mongo `message_logs` row per WhatsApp send, written by
// models.SendMessage with Meta's answer (SUCCESS = Meta accepted it, with its wamid; FAILED = Meta's error).
// Delivery / read receipts are NOT here: Meta's webhooks go to mds-orders, not to this service.
//
// For the console's WhatsApp Templates > Sent log. Phones are masked in every answer; the variables of OTP /
// authentication templates are replaced by "••••". The old open /message-logs route is left as it was.
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
	ist        = time.FixedZone("IST", 5*3600+1800)
	nonDigit   = regexp.MustCompile(`\D`)
	otpNameRe  = regexp.MustCompile(`(?i)otp|verif|passcode|login_code|auth`)
	dayRe      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	indexOnce  sync.Once
	maxDays    = 31
	statusWord = map[string]string{"sent": "SUCCESS", "failed": "FAILED"}
)

// MaskPhone keeps the first two and last three digits: 98•••••243.
func MaskPhone(p string) string {
	d := nonDigit.ReplaceAllString(p, "")
	if len(d) <= 5 {
		return strings.Repeat("•", len(d))
	}
	return d[:2] + strings.Repeat("•", len(d)-5) + d[len(d)-3:]
}

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
}

// Row is one send as the console shows it.
type Row struct {
	ID        string         `json:"id"`
	Time      time.Time      `json:"time"`
	Template  string         `json:"template"`
	To        string         `json:"to"`
	Variables []string       `json:"variables"`
	Status    string         `json:"status"` // sent | failed
	MessageID string         `json:"message_id,omitempty"`
	Error     string         `json:"error,omitempty"`
	OtpMasked bool           `json:"otp_masked"`
	Body      map[string]any `json:"body"`
	Response  map[string]any `json:"response"`
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
		ID: d.ID.Hex(), Time: d.CreatedAt, Template: d.Template, To: MaskPhone(d.Phone),
		Variables: []string{}, OtpMasked: otp, Body: body, Response: resp, Status: "failed",
	}
	if d.Status == "SUCCESS" {
		r.Status = "sent"
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
	// Meta echoes the number back: mask it there too
	for _, c := range asList(resp["contacts"]) {
		cm := asMap(c)
		for _, k := range []string{"input", "wa_id"} {
			if s := asStr(cm[k]); s != "" {
				cm[k] = MaskPhone(s)
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
	if s, ok := statusWord[status]; ok {
		q["status"] = s
	}
	return q
}

// List: GET /notify-logs?from=&to=&template=&phone=&status=sent|failed&page=&limit=
func List(ctx *gin.Context) {
	if db.MessageLogModel == nil {
		ctx.JSON(503, gin.H{"message": "Message log store not connected"})
		return
	}
	indexOnce.Do(func() {
		_, err := db.MessageLogModel.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: -1}}})
		if err != nil {
			log.Println("[notify-logs] created_at index:", err)
		}
	})

	start, end, bad := DayRange(ctx.Query("from"), ctx.Query("to"), time.Now())
	if bad != "" {
		ctx.JSON(400, gin.H{"message": bad})
		return
	}
	status := ctx.Query("status")
	if status != "" && statusWord[status] == "" {
		ctx.JSON(400, gin.H{"message": "Status: sent or failed"})
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
	base := BuildFilter(start, end, template, phone, "")
	counts := gin.H{}
	for word, stored := range statusWord {
		q := bson.M{}
		for k, v := range base {
			q[k] = v
		}
		q["status"] = stored
		n, _ := db.MessageLogModel.CountDocuments(c, q)
		counts[word] = n
	}

	ctx.JSON(200, gin.H{
		"result": rows, "total": total, "page": page, "limit": limit, "counts": counts,
		"from": start.Format("2006-01-02"), "to": end.AddDate(0, 0, -1).Format("2006-01-02"),
	})
}
