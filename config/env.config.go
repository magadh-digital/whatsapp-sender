package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

var (
	WHATSAPP_AUTH_TOKEN string
	WHATSAPP_URL        string

	// MongoDB
	MONGO_URI string
	DB_NAME   string

	// Redis
	REDIS_URI string
)

func LoadEnv() {
	godotenv.Load()

	WHATSAPP_AUTH_TOKEN = os.Getenv("WHATSAPP_AUTH")
	WHATSAPP_URL = os.Getenv("WHATSAPP_URL")
	DB_NAME = os.Getenv("DB_NAME")

	MONGO_URI = os.Getenv("MONGO_URI")
	REDIS_URI = os.Getenv("REDIS_URI")

	var envs = []string{
		WHATSAPP_AUTH_TOKEN, WHATSAPP_URL, DB_NAME, MONGO_URI, REDIS_URI,
	}

	for _, env := range envs {
		if env == "" {
			log.Fatal("Environment variable not set", env)
		}
	}

}
