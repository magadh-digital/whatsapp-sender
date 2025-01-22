package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type EnvConfig struct {
	WHATSAPP_AUTH_TOKEN string `json:"whatsapp_auth_token"`
	WHATSAPP_URL        string `json:"whatsapp_url"`
	MONGO_URI           string `json:"mongo_uri"`
	DB_NAME             string `json:"db_name"`
	REDIS_URI           string `json:"redis_uri"`
	PORT                string `json:"port"`
}

var envConfig EnvConfig = EnvConfig{}

func GetEnvConfig() EnvConfig {
	if envConfig == (EnvConfig{}) {
		godotenv.Load()

		envConfig = EnvConfig{
			WHATSAPP_AUTH_TOKEN: os.Getenv("WHATSAPP_AUTH"),
			WHATSAPP_URL:        os.Getenv("WHATSAPP_URL"),
			MONGO_URI:           os.Getenv("MONGO_URI"),
			DB_NAME:             os.Getenv("DB_NAME"),
			REDIS_URI:           os.Getenv("REDIS_URI"),
			PORT:                os.Getenv("PORT"),
		}

		var envs = []string{
			envConfig.WHATSAPP_AUTH_TOKEN,
			envConfig.WHATSAPP_URL,
			envConfig.MONGO_URI,
			envConfig.DB_NAME,
			envConfig.REDIS_URI,
			envConfig.PORT,
		}

		for _, env := range envs {
			if env == "" {
				log.Fatal("Environment variable not set", env)
			}
		}

	}

	return envConfig
}
