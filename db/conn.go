package db

import (
	"context"
	"log"
	"whatsapp-sender/config"
	"whatsapp-sender/constants"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var DB *mongo.Database

func ConnectDB() {

	// Set client options
	clientOptions := options.Client().ApplyURI(config.GetEnvConfig().MONGO_URI)

	// clientOptions.SetMaxPoolSize(100)
	// clientOptions.SetMaxConnIdleTime(10)

	// Connect to MongoDB
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	// Check the connection
	err = client.Ping(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	DB = client.Database(config.GetEnvConfig().DB_NAME)

	log.Println("Connected to MongoDB!")

}

func RegisterModels(args ...string) {
	for _, arg := range args {
		switch arg {
		case constants.WhatsappTemplateCollection:
			WhatsappTemplateModel = DB.Collection(constants.WhatsappTemplateCollection)

		case constants.MessageLogCollection:
			MessageLogModel = DB.Collection(constants.MessageLogCollection)

		}
	}
}

var (
	// all models
	WhatsappTemplateModel *mongo.Collection
	MessageLogModel       *mongo.Collection
)

func CloseDB() {
	DB.Client().Disconnect(context.Background())
	log.Println("Connection to MongoDB closed.")
}
