package repository

import (
	"context"
	"time"
)

// LoginAttemptRepository はログイン失敗の回数と、一時停止の状態を持つ。
type LoginAttemptRepository interface {
	IsLocked(ctx context.Context, key string) (bool, error)
	// RecordFailure は失敗を1回数え、window の間に maxFailures に達したら key を lockFor の間止める。
	RecordFailure(ctx context.Context, key string, maxFailures int, window, lockFor time.Duration) error
	Clear(ctx context.Context, key string) error
}
