package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

const sseTicketKeyPrefix = "sse_ticket:"

// sseTicketTTL はチケットの寿命。
//
// 使い道は「クライアントがミューテーションで発行 → そのまま EventSource を張る」
// の1往復だけなので、本来は数秒で足りる。30秒にしてあるのは、発行直後にタブが
// バックグラウンドへ回ったりネットワークが一瞬詰まったりしても初回接続が通るように
// するため。これ以上長くすると、アクセスログに残ったチケットが「まだ使える」時間が
// 伸びるだけで、得るものが無い。
const sseTicketTTL = 30 * time.Second

var _ repository.SSETicketRepository = &RedisSSETicketRepository{}

type RedisSSETicketRepository struct {
	client *redis.Client
}

func NewRedisSSETicketRepository(client *redis.Client) repository.SSETicketRepository {
	return &RedisSSETicketRepository{client: client}
}

func (r *RedisSSETicketRepository) Issue(ctx context.Context, ticket string, userID int64) error {
	return r.client.Set(ctx, sseTicketKeyPrefix+ticket, strconv.FormatInt(userID, 10), sseTicketTTL).Err()
}

func (r *RedisSSETicketRepository) Consume(ctx context.Context, ticket string) (int64, bool, error) {
	// GETDEL は Redis 6.2 以降のコマンドで、取得と削除が1コマンド＝不可分に走る
	// （compose の redis は 7-alpine）。GET してから DEL する実装にすると、その隙間に
	// 同じチケットで2本目が張れてしまい「使い捨て」が保証できない。
	raw, err := r.client.GetDel(ctx, sseTicketKeyPrefix+ticket).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		// 保存時に必ず10進整数で書いているので通常起きない。壊れた値は
		// 「無効なチケット」として扱う（GETDEL 済みなので既に消えている）。
		return 0, false, nil
	}
	return userID, true, nil
}
