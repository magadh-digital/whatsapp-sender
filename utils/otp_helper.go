package utils

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"time"
	"whatsapp-sender/models"
	"whatsapp-sender/redis"
)

func GenerateOTP(phone, service string, length uint) string {
	key := redis.OtpMessage + ":" + phone + ":" + service

	// check if otp already exists
	otp, _ := redis.RedisClient.Get(redis.RedisClient.Context(), key).Result()

	if otp != "" {
		return otp
	}

	if length < 4 {
		length = 4
	}

	for i := 0; i < int(length); i++ {
		randomInt, _ := rand.Int(rand.Reader, big.NewInt(10))
		otp += randomInt.String()
	}

	// save otp in redis
	redis.RedisClient.Set(redis.RedisClient.Context(), key, otp, time.Minute*5)

	return otp
}

func ValidateOtp(phone, service, otp string) bool {
	key := redis.OtpMessage + ":" + phone + ":" + service

	// check if otp already exists
	otpInRedis, _ := redis.RedisClient.Get(redis.RedisClient.Context(), key).Result()

	if otpInRedis == otp {
		redis.RedisClient.Del(redis.RedisClient.Context(), key)
		return true
	}

	return false
}

// var message = "Your OTP is: "

//  const url = 'https://api.textlocal.in/send/';
//   const SMS_API = process.env.SMS_API;
//   const SMS_SENDER = process.env.SMS_SENDER;

const (
	SMS_API    = "NmY3OTMwNzQ3MjRjMzQ0NjUxNzk0NDM0NTk3MTU2Nzg="
	SMS_SENDER = "MGDHin"
)

func generateMessage(otp, service string) string {
	return "Dear User,\n\nOTP to log in is " + otp + ", Or use this link to log into app " + service + "\nThank You for using online services.\n\nMagadh Industries"
}

func CallOtpApi(phone, service, otp string) error {
	message := generateMessage(otp, service)

	query := map[string]string{
		"apikey":  SMS_API,
		"message": message,
		"sender":  SMS_SENDER,
		"numbers": phone,
	}

	req := NewApiRequest().SetUrl("https://api.textlocal.in/send/").SetMethod(http.MethodPost)

	for key, value := range query {
		req.SetQuery(key, value)
	}

	res, err := req.Send()

	if err != nil {
		fmt.Println("Error in sending otp", err)
		return err
	}

	log.Println("Response from otp api", res.JsonBody)

	models.SaveInDb(phone, res.JsonBody, "success")

	return nil
}
