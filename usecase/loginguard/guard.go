// Package loginguard は、同じアカウントへのパスワード総当たりを止める。
//
// GraphQL の速度制限は IP や利用者ごとの回数しか見ないので、学内 Wi-Fi のように IP を共有している
// 所からの試行や、IP を変えながらの試行では、1つのアカウントを狙われても止まらない。
package loginguard

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const (
	MaxFailures = 10
	Window      = 15 * time.Minute
	// 第三者がわざと失敗して本人を締め出すこともできるので、止める時間は短くしてある。
	// 10回失敗すると5分止まるなら、総当たりは1日に約2900回までしか試せない。
	LockFor = 5 * time.Minute
)

var (
	// ErrInvalidCredentials は回数に数える失敗。アカウントの有無を区別しないこと。
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTooManyAttempts    = errors.New("too many login attempts")
)

type Guard struct {
	repo repository.LoginAttemptRepository
}

func New(repo repository.LoginAttemptRepository) *Guard {
	return &Guard{repo: repo}
}

// Attempt は止められていなければ login を実行し、結果に応じて失敗の回数を更新する。
// scope は利用者と管理者で回数を分けるためのもの。
//
// Redis が落ちているときは通す。この仕組みのためにログインを全員止めることはしない。
func Attempt[T any](ctx context.Context, g *Guard, scope, email string, login func() (T, error)) (T, error) {
	var zero T
	key := scope + ":" + strings.ToLower(strings.TrimSpace(email))

	locked, err := g.repo.IsLocked(ctx, key)
	if err != nil {
		logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to read lock")
	} else if locked {
		return zero, ErrTooManyAttempts
	}

	result, err := login()
	switch {
	case err == nil:
		if err := g.repo.Clear(ctx, key); err != nil {
			logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to clear failures")
		}
	case errors.Is(err, ErrInvalidCredentials):
		if err := g.repo.RecordFailure(ctx, key, MaxFailures, Window, LockFor); err != nil {
			logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to record failure")
		}
	}
	return result, err
}
