package redis

import (
	"fmt"
	"strings"

	"notify/config"

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
	uri := config.GetEnvConfig().REDIS_URI
	if uri == "" {
		uri = "localhost:6379"
	}

	var rdb *redis.Client

	// Supports both "localhost:6379" and "redis://localhost:6379"
	if strings.HasPrefix(uri, "redis://") || strings.HasPrefix(uri, "rediss://") {
		opts, err := redis.ParseURL(uri)
		if err != nil {
			fmt.Println("Error parsing REDIS_URI:", err)
			panic(err)
		}
		rdb = redis.NewClient(opts)
	} else {
		rdb = redis.NewClient(&redis.Options{
			Addr: uri,
		})
	}

	_, err := rdb.Ping(rdb.Context()).Result()
	if err != nil {
		fmt.Println("Error connecting to Redis:", err)
		panic(err)
	}

	fmt.Println("Connected to Redis")
	return rdb
}
