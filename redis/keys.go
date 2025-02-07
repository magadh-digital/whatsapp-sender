package redis

import (
	"fmt"
	"reflect"
)

const prefix = "whatsapp-sender"

var RedisKeys = struct {
	WhatsappMessageQueue string
	Counter              string
	OtpMessage           string
	ConstantCache        string
}{
	WhatsappMessageQueue: "message-queue",
	Counter:              "counter",
	OtpMessage:           "otp",
	ConstantCache:        "constants",
}

func init() {
	t := reflect.TypeOf(RedisKeys)

	for i := 0; i < t.NumField(); i++ {
		value := reflect.ValueOf(&RedisKeys).Elem().Field(i).String()
		reflect.ValueOf(&RedisKeys).Elem().Field(i).SetString(prefix + ":" + value)
	}

	fmt.Println(RedisKeys)
}
