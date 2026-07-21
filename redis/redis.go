package redis

import (
	"fmt"
	"os"

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

func getRedisAddr() string {
	addr := os.Getenv("REDIS_URI")
	if addr == "" {
		addr = "localhost:6379"
	}
	return addr
}

func GetRedisClient() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: getRedisAddr(),
	})

	_, err := rdb.Ping(rdb.Context()).Result()
	if err != nil {
		fmt.Println("Error connecting to Redis:", err)
		panic(err)
	}

	fmt.Println("Connected to Redis at", getRedisAddr())
	return rdb
}
