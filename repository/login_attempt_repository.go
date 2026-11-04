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
	// Clear は失敗の回数と一時停止の両方を消す。
	Clear(ctx context.Context, key string) error
	// RememberIP は key のアカウントに ip からログインできたことを ttl の間覚える。
	RememberIP(ctx context.Context, key, ip string, ttl time.Duration) error
	// IsKnownIP は ip が RememberIP で覚えた IP かどうか。
	IsKnownIP(ctx context.Context, key, ip string) (bool, error)
}
