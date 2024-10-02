package models

import (
	"context"
	"fmt"
	"time"
	"whatsapp-sender/db"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
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
