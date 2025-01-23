package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"whatsapp-sender/models"
	"whatsapp-sender/redis"
)

type QueuePayload struct {
	Payload *models.WhatsappTemplate
	Phone   string
}

func QueueMessage(payload *models.WhatsappTemplate, phone []string) {

	payloadList := make([]string, 0)

	for i := range phone {

		queuePayload := QueuePayload{
			Payload: payload,
			Phone:   phone[i],
		}

		// marshal the payload
		jsonPayload, err := json.Marshal(queuePayload)

		if err != nil {
			fmt.Println("Error in marshalling payload", err)
			continue
		}

		payloadList = append(payloadList, string(jsonPayload))

	}

	if len(payloadList) > 0 {
		result := redis.RedisClient.RPush(context.Background(), redis.RedisKeys.WhatsappMessageQueue, payloadList).Val()
		fmt.Println("Total message requested ", len(payloadList), "Total message queued", result)
	}

}

func QueueProcessing() {
	concurrent := 30

	queue := make(chan QueuePayload, concurrent)

	for i := 0; i < concurrent; i += 1 {
		go func() {
			for {
				queuePayload := <-queue

				payload := queuePayload.Payload
				phone := queuePayload.Phone

				models.SendMessage(payload, phone)

				// save the message in database
				fmt.Println("Message sent to ", phone)
			}
		}()
	}

	go func() {
		client := redis.GetRedisClient()

		defer client.Close()

		for {
			result := client.BLPop(client.Context(), 0, redis.RedisKeys.WhatsappMessageQueue).Val()

			value := result[1]

			if value != "" {
				var payload QueuePayload
				err := json.Unmarshal([]byte(value), &payload)

				if err != nil {
					fmt.Println("Error in unmarshalling payload", err)
					continue
				}

				queue <- payload
			}
		}
	}()

	// for i := 0; i < concurrent; i += 1 {
	// 	go func() {
	// 		client := redis.GetRedisClient()

	// 		defer client.Close()

	// 		for {
	// 			result := client.BLPop(client.Context(), 0, redis.WhatsappMessageQueue).Val()

	// 			value := result[1]

	// 			if value != "" {
	// 				var payload QueuePayload
	// 				err := json.Unmarshal([]byte(value), &payload)

	// 				if err != nil {
	// 					// TODO: Log the error in database
	// 					fmt.Println("Error in unmarshalling payload", err)
	// 					continue
	// 				}

	// 				models.SendMessage(payload.Payload, payload.Phone)

	// 				// save the message in database
	// 				fmt.Println("Message sent to ", payload.Phone)

	// 			}
	// 		}
	// 	}()
	// }
}
