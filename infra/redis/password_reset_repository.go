package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

type RedisPasswordResetRepository struct {
	client *redis.Client
}

func NewRedisPasswordResetRepository(client *redis.Client) repository.PasswordResetRepository {
	return &RedisPasswordResetRepository{client: client}
}

func (r *RedisPasswordResetRepository) SaveOTP(ctx context.Context, email, otp string, ttl time.Duration) error {
	return r.client.Set(ctx, "pwreset:otp:"+email, otp, ttl).Err()
}

func (r *RedisPasswordResetRepository) GetOTP(ctx context.Context, email string) (string, error) {
	val, err := r.client.Get(ctx, "pwreset:otp:"+email).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (r *RedisPasswordResetRepository) DeleteOTP(ctx context.Context, email string) error {
	return r.client.Del(ctx, "pwreset:otp:"+email).Err()
}

func (r *RedisPasswordResetRepository) SaveResetToken(ctx context.Context, token, email string, ttl time.Duration) error {
	return r.client.Set(ctx, "pwreset:token:"+token, email, ttl).Err()
}

func (r *RedisPasswordResetRepository) GetEmailByResetToken(ctx context.Context, token string) (string, error) {
	val, err := r.client.Get(ctx, "pwreset:token:"+token).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (r *RedisPasswordResetRepository) DeleteResetToken(ctx context.Context, token string) error {
	return r.client.Del(ctx, "pwreset:token:"+token).Err()
}

func (r *RedisPasswordResetRepository) SetPasswordChangedAt(ctx context.Context, userID int64, t time.Time, ttl time.Duration) error {
	key := fmt.Sprintf("pwreset:changed:%d", userID)
	return r.client.Set(ctx, key, strconv.FormatInt(t.Unix(), 10), ttl).Err()
}

func (r *RedisPasswordResetRepository) GetPasswordChangedAt(ctx context.Context, userID int64) (*time.Time, error) {
	key := fmt.Sprintf("pwreset:changed:%d", userID)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	unix, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return nil, err
	}
	t := time.Unix(unix, 0)
	return &t, nil
}
