package redis

import (
	"context"
	"math"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"
)

// rateLimiterKeyPrefix は速度制限のバケツを置くキーの頭。
const rateLimiterKeyPrefix = "ratelimit:"

// rateLimiterScript はトークンバケツ1回ぶんの出し入れ。
//
// 「読んで、計算して、書く」を Redis の中で1回に閉じる。Go 側で読み書きに
// 分けると、同じIPからの同時リクエストが互いの書き込みを踏み、制限をすり抜ける。
//
// 経過時間が負になる場合（台ごとの時計のずれ）は補充しない。補充してしまうと、
// 時計が進んでいる台を選んで叩くだけで無限にトークンを作れる。
var rateLimiterScript = redis.NewScript(`
local key   = KEYS[1]
local rate  = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now   = tonumber(ARGV[3])
local ttl   = tonumber(ARGV[4])

local bucket = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(bucket[1])
local ts     = tonumber(bucket[2])

if tokens == nil or ts == nil then
  tokens = burst
  ts = now
end

local elapsed = now - ts
if elapsed > 0 then
  tokens = math.min(burst, tokens + elapsed * rate)
  ts = now
end

local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

redis.call('HSET', key, 'tokens', tokens, 'ts', ts)
redis.call('EXPIRE', key, ttl)
return allowed
`)

// RateLimiterStore は速度制限のカウントを Redis に置く echo の RateLimiterStore。
//
// 既定のメモリ実装はプロセスごとに数えるので、台を N 個立てると 1IP あたり
// 実質 N 倍まで通ってしまう。攻撃側から見れば台数を増やすほど制限が緩む。
// ここに置けば、台が何個あっても 1IP あたりの上限は1つになる。
//
// Redis が応答しないときは通す。速度制限は本来の処理を守るための添え物であって、
// これを理由に全リクエストを落とすと、守るはずの側を自分で止めることになる。
type RateLimiterStore struct {
	client *redis.Client
	prefix string
	rate   float64
	burst  float64
	ttl    time.Duration
	// now はテスト用に差し替えられるようにしてある（補充を確かめるのに
	// 実時間を待たずに済ませるため）。本番は NewRateLimiterStore が time.Now を入れる。
	now func() time.Time
}

var _ echoMiddleware.RateLimiterStore = (*RateLimiterStore)(nil)

// NewRateLimiterStore は rate（1秒あたりの補充数）と burst（バケツの大きさ）で
// 速度制限の置き場を作る。
func NewRateLimiterStore(client *redis.Client, prefix string, rate, burst float64) *RateLimiterStore {
	return &RateLimiterStore{
		client: client,
		prefix: prefix,
		rate:   rate,
		burst:  burst,
		// バケツが満杯に戻りきるまで持てばよい。それ以降は、残しておいても
		// 作り直しても満杯から始まるので同じ。短くしておくと、一度きりの
		// アクセス元のぶんが Redis に溜まり続けずに済む。
		ttl: time.Duration(math.Ceil(burst/rate))*time.Second + time.Second,
		now: time.Now,
	}
}

func (s *RateLimiterStore) Allow(identifier string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	now := float64(s.now().UnixNano()) / float64(time.Second)
	res, err := rateLimiterScript.Run(
		ctx, s.client,
		[]string{s.prefix + rateLimiterKeyPrefix + identifier},
		s.rate, s.burst, now, int(s.ttl.Seconds()),
	).Int64()
	if err != nil {
		logger.Log.Error().Err(err).
			Str("component", "redis_rate_limiter").
			Msg("rate limit check failed; letting the request through")
		return true, nil
	}
	return res == 1, nil
}
