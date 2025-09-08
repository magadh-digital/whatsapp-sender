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
	SMS_API_KEY             string `json:"sms_api"`
	SMS_API_TOKEN          string `json:"sms_api_token"`
	SMS_SUBDOMAIN			 string `json:"sms_subdomain"`
	SMS_SID				 string `json:"sms_sid"`			 
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
			SMS_API_KEY:             os.Getenv("SMS_API_KEY"),
			SMS_API_TOKEN:       os.Getenv("SMS_API_TOKEN"),
			SMS_SUBDOMAIN:       os.Getenv("SMS_SUBDOMAIN"),
			SMS_SID:            os.Getenv("SMS_SID"),

		}

		var envs = []string{
			envConfig.WHATSAPP_AUTH_TOKEN,
			envConfig.WHATSAPP_URL,
			envConfig.MONGO_URI,
			envConfig.DB_NAME,
			envConfig.REDIS_URI,
			envConfig.PORT,
			envConfig.SMS_API_KEY,
			envConfig.SMS_API_TOKEN,
			envConfig.SMS_SUBDOMAIN,
			envConfig.SMS_SID,

		}

		for i, env := range envs {
			if env == "" {
				log.Fatal("Environment variable not set", i)
			}
		}

	}

	return envConfig
}
