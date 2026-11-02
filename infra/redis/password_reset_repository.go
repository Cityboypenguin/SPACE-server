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

func resetOTPAttemptsKey(email string) string {
	return "pwreset:attempts:" + strings.ToLower(strings.TrimSpace(email))
}

// otpMaxFailures は1つの認証コードを試せる回数。6桁しかないので、速度制限だけでは10分の間に
// 当てられてしまう。超えたらコードを捨てて、メールで取り直してもらう。
const otpMaxFailures = 5

var exchangeResetOTPScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then
  return 0
end
if current == ARGV[1] then
  redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
  redis.call('DEL', KEYS[1], KEYS[3])
  return 1
end
local failures = redis.call('INCR', KEYS[3])
if failures == 1 then
  redis.call('PEXPIRE', KEYS[3], redis.call('PTTL', KEYS[1]))
end
if failures >= tonumber(ARGV[4]) then
  redis.call('DEL', KEYS[1], KEYS[3])
end
return 0`)

func (r *RedisPasswordResetRepository) SaveOTP(ctx context.Context, email, otp string, ttl time.Duration) error {
	if err := r.client.Del(ctx, resetOTPAttemptsKey(email)).Err(); err != nil {
		return err
	}
	return r.client.Set(ctx, resetOTPKey(email), otp, ttl).Err()
}

func (r *RedisPasswordResetRepository) ExchangeOTPForResetToken(ctx context.Context, email, otp, token string, ttl time.Duration) (bool, error) {
	n, err := exchangeResetOTPScript.Run(ctx, r.client,
		[]string{resetOTPKey(email), "pwreset:token:" + token, resetOTPAttemptsKey(email)}, otp, email, ttl.Milliseconds(), otpMaxFailures).Int()
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
