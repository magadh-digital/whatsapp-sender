package redis

const prefix = "whatsapp-sender"

var RedisKeys = struct {
	WhatsappMessageQueue string
	Counter              string
	OtpMessage           string
	ConstantCache        string
}{
	WhatsappMessageQueue: prefix + ":message-queue",
	Counter:              prefix + ":counter",
	OtpMessage:           prefix + ":otp",
	ConstantCache:        prefix + ":constants",
}
