package redis

const prefix = "whatsapp-sender"

const (
	WhatsappMessageQueue = prefix + ":message-queue"
	Counter              = prefix + ":counter"
	OtpMessage           = prefix + ":otp"
)
