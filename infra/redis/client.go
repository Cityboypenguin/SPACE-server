package redis

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

func New() (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%s",
		getenv("REDIS_HOST", "localhost"),
		getenv("REDIS_PORT", "6379"),
	)

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	return client, nil
}

func getenv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
