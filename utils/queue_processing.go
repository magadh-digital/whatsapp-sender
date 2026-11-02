package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"notify/models"
	"notify/redis"
)

// QueuePayload is one queued send: the filled template, the phone and the caller's batch id (kept on the log row).
type QueuePayload struct {
	Payload *models.WhatsappTemplate
	Phone   string
	Ref     string `json:",omitempty"`
}

func QueueMessage(payload *models.WhatsappTemplate, phone []string) {
	QueueMessageRef(payload, phone, "")
}

// QueueMessageRef pushes one job per phone onto the Redis list the workers drain; `ref` travels with each job.
func QueueMessageRef(payload *models.WhatsappTemplate, phone []string, ref string) {

	payloadList := make([]string, 0)

	for i := range phone {

		queuePayload := QueuePayload{
			Payload: payload,
			Phone:   phone[i],
			Ref:     ref,
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

	for range concurrent {
		go func() {
			for {
				queuePayload := <-queue

				models.SendMessageRef(queuePayload.Payload, queuePayload.Phone, queuePayload.Ref)

				// save the message in database
				fmt.Println("Message sent to ", queuePayload.Phone)
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
}
