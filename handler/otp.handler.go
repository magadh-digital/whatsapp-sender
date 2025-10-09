package handler

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"whatsapp-sender/redis"
	"whatsapp-sender/utils"

	"github.com/gin-gonic/gin"
)

type OtpDto struct {
	Phone   string `json:"phone" binding:"required"`
	Length  uint   `json:"length" `
	Otp     string `json:"otp" `
	Service string `json:"service" binding:"required"`
}

type OtpValidationDto struct {
	Phone   string `json:"phone" binding:"required"`
	Service string `json:"service" binding:"required"`
	Otp     string `json:"otp" binding:"required"`
}

func SendOTP(c *gin.Context) {
	var data OtpDto

	if err := c.ShouldBind(&data); err != nil {
		c.JSON(400, gin.H{
			"error":   err.Error(),
			"message": "Invalid request",
		})
		return
	}

	phoneList := strings.Split(data.Phone, ",")

	if data.Length > 9 {
		data.Otp = fmt.Sprintf("%d", data.Length)
	}

	// if length is 0 then set to 4
	if data.Length == 0 {
		data.Length = 4
	}

	// generate otp
	if data.Otp == "" {
		// generate random otp of length data.Length
		otp := ""
		for i := 0; i < int(data.Length); i++ {
			n, _ := rand.Int(rand.Reader, big.NewInt(10))
			otp += n.String()
		}
		data.Otp = otp
	}

	otp := utils.GenerateOTP(phoneList, data.Service, data.Otp)

	err := utils.CallOtpApi(data.Phone, data.Service, otp)

	if err != nil {
		c.JSON(500, gin.H{
			"message": "Error in sending OTP",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"message": "OTP sent successfully",
		"otp":     otp,
	})
}

func ValidateOtp(c *gin.Context) {

	var data OtpValidationDto

	if err := c.ShouldBind(&data); err != nil {
		c.JSON(400, gin.H{
			"error":   err.Error(),
			"message": "Invalid Body",
		})
		return
	}

	if utils.ValidateOtp(data.Phone, data.Service, data.Otp) {
		c.JSON(200, gin.H{
			"message": "OTP is valid",
		})
		return
	}

	c.JSON(400, gin.H{
		"message": "Invalid OTP",
	})

}

type GetOtpDto struct {
	Phone   string `form:"phone" binding:"required"`
	Service string `form:"service" binding:"required"`
}

func Getotp(c *gin.Context) {

	// get otp from redis
	phone := c.Query("phone")
	service := c.Query("service")
	if phone == "" || service == "" {
		c.JSON(400, gin.H{
			"message": "Phone and Service are required",
		})
		return
	}
	key := redis.RedisKeys.OtpMessage + ":" + phone + ":" + service
	fmt.Println("Getting otp key", key)
	otp, err := redis.RedisClient.Get(redis.RedisClient.Context(), key).Result()
	if err != nil {
		c.JSON(500, gin.H{
			"message": "Error in getting OTP",
			"error":   err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"message": "OTP fetched successfully",
		"otp":     otp,
	})

}
