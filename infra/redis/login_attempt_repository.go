package redis

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

type RedisLoginAttemptRepository struct {
	client *redis.Client
}

func NewRedisLoginAttemptRepository(client *redis.Client) repository.LoginAttemptRepository {
	return &RedisLoginAttemptRepository{client: client}
}

func loginFailureKey(key string) string { return "login:fail:" + key }
func loginLockKey(key string) string    { return "login:lock:" + key }

var recordLoginFailureScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
if n >= tonumber(ARGV[1]) then
  redis.call('SET', KEYS[2], '1', 'PX', ARGV[3])
  redis.call('DEL', KEYS[1])
end
return n`)

func (r *RedisLoginAttemptRepository) IsLocked(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, loginLockKey(key)).Result()
	return n > 0, err
}

func (r *RedisLoginAttemptRepository) RecordFailure(ctx context.Context, key string, maxFailures int, window, lockFor time.Duration) error {
	return recordLoginFailureScript.Run(ctx, r.client,
		[]string{loginFailureKey(key), loginLockKey(key)},
		maxFailures, window.Milliseconds(), lockFor.Milliseconds()).Err()
}

func (r *RedisLoginAttemptRepository) Clear(ctx context.Context, key string) error {
	return r.client.Del(ctx, loginFailureKey(key)).Err()
}
