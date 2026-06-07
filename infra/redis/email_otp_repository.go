package redis

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

const otpKeyPrefix = "email_otp:"
const otpTTL = 10 * time.Minute

var _ repository.EmailOTPRepository = &RedisEmailOTPRepository{}

type RedisEmailOTPRepository struct {
	client *redis.Client
}

func NewRedisEmailOTPRepository(client *redis.Client) repository.EmailOTPRepository {
	return &RedisEmailOTPRepository{client: client}
}

func (r *RedisEmailOTPRepository) Save(ctx context.Context, otp *model.EmailOTP) error {
	return r.client.Set(ctx, otpKeyPrefix+otp.Email, otp.Code, otpTTL).Err()
}

func (r *RedisEmailOTPRepository) FindLatestByEmail(ctx context.Context, email string) (*model.EmailOTP, error) {
	code, err := r.client.Get(ctx, otpKeyPrefix+email).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ttl, err := r.client.TTL(ctx, otpKeyPrefix+email).Result()
	if err != nil {
		return nil, err
	}
	return &model.EmailOTP{
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

func (r *RedisEmailOTPRepository) Delete(ctx context.Context, email string) error {
	return r.client.Del(ctx, otpKeyPrefix+email).Err()
}
