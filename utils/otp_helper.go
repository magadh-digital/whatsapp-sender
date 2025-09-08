package utils

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
	"whatsapp-sender/config"
	"whatsapp-sender/models"
	"whatsapp-sender/redis"
)

func GenerateOTP(phoneList []string, service string, length uint) string {

	phone := phoneList[0]
	key := redis.RedisKeys.OtpMessage + ":" + phone + ":" + service
	fmt.Println("Generating otp key", key)

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

	for _, phone := range phoneList {
		key := redis.RedisKeys.OtpMessage + ":" + phone + ":" + service
		redis.RedisClient.Set(redis.RedisClient.Context(), key, otp, time.Minute*5)
	}

	return otp
}

func ValidateOtp(phone, service, otp string) bool {
	key := redis.RedisKeys.OtpMessage + ":" + phone + ":" + service
	fmt.Println("Validating otp key", key)

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

// data.append('Body', 'Dear User,  OTP to log in is 1234, Or use this link to log into app Magadh APP Thank You for using online services.  Magadh Industries');

// func generateMessage(otp, service string) string {
// 	return "Dear User,\n\nOTP to log in is " + otp + ", Or use this link to log into app " + service + "\nThank You for using online services.\n\nMagadh Industries"
// }

func generateMessage(otp, service string) string {
	return "Dear User, OTP to log in is " + otp + ", Or use this link to log into app Magadh APP Thank You for using online services. Magadh Industries"

}

var (
	SMS_API_KEY   = config.GetEnvConfig().SMS_API_KEY
	SMS_API_TOKEN = config.GetEnvConfig().SMS_API_TOKEN
	SMS_SUBDOMAIN = config.GetEnvConfig().SMS_SUBDOMAIN
	SMS_SID       = config.GetEnvConfig().SMS_SID
)

func CallOtpApi(phone, service, otp string) error {
	message := generateMessage(otp, service)

	endpoint := fmt.Sprintf("https://%s/v1/Accounts/%s/Sms/send", SMS_SUBDOMAIN, SMS_SID)

	// form data
	data := url.Values{}
	data.Set("From", SMS_SENDER)
	data.Set("To", phone)
	data.Set("Body", message)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		fmt.Println("Error in sending otp", err)
		return err
	}

	req.SetBasicAuth(SMS_API_KEY, SMS_API_TOKEN)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		fmt.Println("Error in sending otp", err)
		return err
	}
	defer res.Body.Close()
	// log response
	log.Println("Response from otp api", res.Body)

	responseBody, _ := io.ReadAll(res.Body)
	fmt.Println(string(responseBody))

	// save in db
	err = models.SaveInDb(phone, string(responseBody), "success")
	if err != nil {
		fmt.Println("error in saving in db", err)
	}
	fmt.Println("saved in db")

	return nil
}
