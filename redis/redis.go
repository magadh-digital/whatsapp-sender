// github.com/go-redis/redis/v9

package redis

import (
	"fmt"

	"github.com/go-redis/redis/v8"
)

var RedisClient *redis.Client

func ConnectToRedis() {
	RedisClient = GetRedisClient()
}

func DefaultRedisClient() *redis.Client {
	if RedisClient == nil {
		RedisClient = GetRedisClient()
	}

	return RedisClient
}

func GetRedisClient() *redis.Client {

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	_, err := rdb.Ping(rdb.Context()).Result()

	if err != nil {
		fmt.Println("Error connecting to Redis")
		panic(err)
	}

	fmt.Println("Connected to Redis")
	return rdb

}
