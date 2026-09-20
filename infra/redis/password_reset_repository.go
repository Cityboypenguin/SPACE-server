package redis

import (
	"context"
	"strings"
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

func resetOTPKey(email string) string {
	return "pwreset:otp:" + strings.ToLower(strings.TrimSpace(email))
}

var exchangeResetOTPScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
  redis.call('DEL', KEYS[1])
  return 1
end
return 0`)

func (r *RedisPasswordResetRepository) SaveOTP(ctx context.Context, email, otp string, ttl time.Duration) error {
	return r.client.Set(ctx, resetOTPKey(email), otp, ttl).Err()
}

func (r *RedisPasswordResetRepository) ExchangeOTPForResetToken(ctx context.Context, email, otp, token string, ttl time.Duration) (bool, error) {
	n, err := exchangeResetOTPScript.Run(ctx, r.client,
		[]string{resetOTPKey(email), "pwreset:token:" + token}, otp, email, ttl.Milliseconds()).Int()
	return n == 1, err
}

func (r *RedisPasswordResetRepository) TryBeginRequest(ctx context.Context, email string, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, "pwreset:rate:"+strings.ToLower(strings.TrimSpace(email)), "1", ttl).Result()
}

func (r *RedisPasswordResetRepository) ConsumeResetToken(ctx context.Context, token string) (string, error) {
	val, err := r.client.GetDel(ctx, "pwreset:token:"+token).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}
