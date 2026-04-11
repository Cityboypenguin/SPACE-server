package redis

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

type RedisRevokedTokenRepository struct {
	client *redis.Client
}

func NewRedisRevokedTokenRepository(client *redis.Client) repository.RevokedTokenRepository {
	return &RedisRevokedTokenRepository{client: client}
}

func (r *RedisRevokedTokenRepository) RevokeToken(ctx context.Context, token string, expiresAt int64) error {
	ttl := time.Until(time.Unix(expiresAt, 0))
	if ttl <= 0 {
		return nil
	}
	return r.client.Set(ctx, "revoked:"+token, "1", ttl).Err()
}

func (r *RedisRevokedTokenRepository) IsRevoked(ctx context.Context, token string) (bool, error) {
	result, err := r.client.Exists(ctx, "revoked:"+token).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}
