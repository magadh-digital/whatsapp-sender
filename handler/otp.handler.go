package handler

import (
	"strings"
	"whatsapp-sender/utils"

	"github.com/gin-gonic/gin"
)

type OtpDto struct {
	Phone   string `json:"phone" binding:"required"`
	Length  uint   `json:"length" `
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

	otp := utils.GenerateOTP(phoneList, data.Service, data.Length)

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
