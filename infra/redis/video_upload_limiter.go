package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// VideoUploadLimiter は動画の署名付きURLを1人が出せる回数を、時間枠ごとに数えて抑える。
//
// 動画の受け入れは中身の検査（先頭 1MB の読み込みと解析）を伴うので、URL を
// 短時間に大量に取られると、それだけ置き場とサーバーに負担がかかる。
// 1日の本数の上限（system_settings）とは別の、取りすぎを止めるための制限。
type VideoUploadLimiter struct {
	client *redis.Client
	limit  int64
	window time.Duration
}

func NewVideoUploadLimiter(client *redis.Client, limit int64, window time.Duration) *VideoUploadLimiter {
	return &VideoUploadLimiter{client: client, limit: limit, window: window}
}

// Allow は今回の発行を数え、枠内の上限を超えていなければ true を返す。
func (l *VideoUploadLimiter) Allow(ctx context.Context, userID int64) (bool, error) {
	bucket := time.Now().Unix() / int64(l.window/time.Second)
	key := fmt.Sprintf("video_upload_url:%d:%d", userID, bucket)
	pipe := l.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, l.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return incr.Val() <= l.limit, nil
}
