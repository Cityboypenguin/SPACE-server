package redis

import (
	"context"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

const otpKeyPrefix = "email_otp:"
const otpRateLimitPrefix = "email_otp_rate:"
const otpTTL = 10 * time.Minute
const otpRateLimitTTL = 1 * time.Minute

var _ repository.EmailOTPRepository = &RedisEmailOTPRepository{}

type RedisEmailOTPRepository struct {
	client *redis.Client
}

func NewRedisEmailOTPRepository(client *redis.Client) repository.EmailOTPRepository {
	return &RedisEmailOTPRepository{client: client}
}

func emailOTPKey(email string) string {
	return otpKeyPrefix + strings.ToLower(strings.TrimSpace(email))
}
func emailOTPRateKey(email string) string {
	return otpRateLimitPrefix + strings.ToLower(strings.TrimSpace(email))
}

func emailOTPAttemptsKey(email string) string {
	return "email_otp_attempts:" + strings.ToLower(strings.TrimSpace(email))
}

func (r *RedisEmailOTPRepository) Save(ctx context.Context, otp *model.EmailOTP) error {
	if err := r.client.Del(ctx, emailOTPAttemptsKey(otp.Email)).Err(); err != nil {
		return err
	}
	return r.client.Set(ctx, emailOTPKey(otp.Email), otp.Code, otpTTL).Err()
}

func (r *RedisEmailOTPRepository) FindLatestByEmail(ctx context.Context, email string) (*model.EmailOTP, error) {
	code, err := r.client.Get(ctx, emailOTPKey(email)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ttl, err := r.client.TTL(ctx, emailOTPKey(email)).Result()
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
	return r.client.Del(ctx, emailOTPKey(email)).Err()
}

var recordEmailOTPFailureScript = redis.NewScript(`
local failures = redis.call('INCR', KEYS[2])
if failures == 1 then
  redis.call('PEXPIRE', KEYS[2], ARGV[2])
end
if failures >= tonumber(ARGV[1]) then
  redis.call('DEL', KEYS[1], KEYS[2])
end
return failures`)

func (r *RedisEmailOTPRepository) RecordFailure(ctx context.Context, email string) error {
	return recordEmailOTPFailureScript.Run(ctx, r.client,
		[]string{emailOTPKey(email), emailOTPAttemptsKey(email)}, otpMaxFailures, otpTTL.Milliseconds()).Err()
}

func (r *RedisEmailOTPRepository) TryBeginSend(ctx context.Context, email string) (bool, error) {
	return r.client.SetNX(ctx, emailOTPRateKey(email), "1", otpRateLimitTTL).Result()
}
