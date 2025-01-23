package models

import (
	"context"
	"fmt"
	"time"
	"whatsapp-sender/db"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Constants struct {
	Title     string    `json:"title" bson:"title"`
	Category  string    `json:"category" bson:"category"`
	SortValue int       `json:"sort_value" bson:"sort_value"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

func ConstantsModel() *mongo.Collection {
	return db.DB.Collection("constants")
}

func CreateConstantModelIndex() {
	result, err := ConstantsModel().Indexes().CreateOne(
		context.Background(),
		mongo.IndexModel{
			Keys: bson.D{
				{Key: "title", Value: 1},
				{Key: "category", Value: 1},
			},

			Options: options.Index().SetUnique(true),
		},
	)

	if err != nil {
		panic(err)
	}

	fmt.Println(result)
}
