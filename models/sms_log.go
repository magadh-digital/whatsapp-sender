package models

import (
	"context"
	"log"
	"time"
	"whatsapp-sender/db"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SmsLog struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Phone        string             `json:"phone" bson:"phone"`
	ResponseBody interface{}        `json:"response_body" bson:"response_body"`
	CreatedAt    time.Time          `json:"created_at" bson:"created_at"`
	Status       string             `json:"status" bson:"status"`
}

func SaveInDb(phone string, responseBody interface{}, status string) error {
	val := &SmsLog{
		Phone:        phone,
		ResponseBody: responseBody,
		Status:       status,
		CreatedAt:    time.Now(),
	}

	_, err := db.DB.Collection("sms_logs").InsertOne(context.Background(), val)

	if err != nil {
		log.Println("Error in saving sms log in db", err.Error())
	}

	return err
}
