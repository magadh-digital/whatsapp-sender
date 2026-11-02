package models

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"notify/db"
	"sync"
	"time"
)

const (
	MessageLogStatusFailed  = "FAILED"
	MessageLogStatusSuccess = "SUCCESS"
)

type MessageLog struct {
	ID        primitive.ObjectID     `bson:"_id,omitempty" json:"id,omitempty"`
	Template  string                 `json:"template" bson:"template"`
	Body      map[string]interface{} `json:"body" bson:"body,omitempty"`
	Payload   map[string]interface{} `json:"payload" bson:"payload,omitempty"`
	Phone     string                 `json:"phone" bson:"phone"`
	Status    string                 `json:"status" bson:"status"`
	CreatedAt time.Time              `json:"created_at" bson:"created_at"`
	// the caller's batch id (a console broadcast's job id), empty for single sends
	Ref string `json:"ref,omitempty" bson:"ref,omitempty"`
}

// WithRef tags the log row with the caller's batch id; an empty ref changes nothing.
func (m *MessageLog) WithRef(ref string) *MessageLog {
	m.Ref = ref
	return m
}

func NewMessageLog(template string, payload map[string]interface{}, phone string, status string, body map[string]interface{}) *MessageLog {

	if payload == nil {
		payload = make(map[string]interface{}, 0)
	}

	return &MessageLog{
		Template:  template,
		Payload:   payload,
		Body:      body,
		Phone:     phone,
		Status:    status,
		CreatedAt: time.Now(),
	}
}

func (m *MessageLog) Create() error {

	value, err := db.MessageLogModel.InsertOne(context.Background(), m)

	if err != nil {
		return err
	}

	m.ID = value.InsertedID.(primitive.ObjectID)

	return nil
}

func GetMessageLogs(query bson.M, option options.FindOptions) ([]MessageLog, error) {
	var logs []MessageLog = make([]MessageLog, 0)

	fmt.Println(query)
	cursor, err := db.MessageLogModel.Find(context.Background(), query, &option)

	if err != nil {
		return logs, err
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var log MessageLog
		err := cursor.Decode(&log)

		if err != nil {
			return logs, err
		}

		logs = append(logs, log)
	}

	return logs, nil
}

func CountMessageLogs(query bson.M) int64 {
	count, _ := db.MessageLogModel.CountDocuments(context.Background(), query)
	return count
}

// TemplateStat is what the log says about one template: how many messages were ever queued under its name and when
// the last one was.
type TemplateStat struct {
	Sent       int64     `json:"sent"`
	LastSentAt time.Time `json:"last_sent_at"`
}

var statsIndexOnce sync.Once

// TemplateStats groups the whole message log by template name (one aggregation; the template list caches it for
// 60 s). A missing collection or a failed aggregation gives an empty map, never an error to the caller.
func TemplateStats() map[string]TemplateStat {
	out := map[string]TemplateStat{}
	if db.MessageLogModel == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	statsIndexOnce.Do(func() {
		_, _ = db.MessageLogModel.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "template", Value: 1}, {Key: "created_at", Value: -1}}})
	})
	cur, err := db.MessageLogModel.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$template"},
			{Key: "sent", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "last", Value: bson.D{{Key: "$max", Value: "$created_at"}}},
		}}},
	})
	if err != nil {
		fmt.Println("template stats:", err)
		return out
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var row struct {
			ID   string    `bson:"_id"`
			Sent int64     `bson:"sent"`
			Last time.Time `bson:"last"`
		}
		if cur.Decode(&row) == nil && row.ID != "" {
			out[row.ID] = TemplateStat{Sent: row.Sent, LastSentAt: row.Last}
		}
	}
	return out
}
