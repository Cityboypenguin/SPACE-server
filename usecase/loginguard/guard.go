// Package loginguard は、同じアカウントへのパスワード総当たりを止める。
//
// GraphQL の速度制限は IP や利用者ごとの回数しか見ないので、学内 Wi-Fi のように IP を共有している
// 所からの試行や、IP を変えながらの試行では、1つのアカウントを狙われても止まらない。
//
// 失敗は2つの単位で数える。
//   - アカウントと IP の組。1つの IP からの総当たりを止める。
//   - アカウント全体。IP を変えながらの総当たりを止める。ただし本人が以前ログインできた IP は
//     ここに数えず、ここで止まってもログインできる。第三者がわざと失敗しても、本人を
//     いつもの場所から締め出せないようにするため。
package loginguard

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/clientip"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"golang.org/x/crypto/bcrypt"
)

const (
	MaxFailures = 10
	Window      = 15 * time.Minute
	// 10回失敗すると5分止まるなら、総当たりは1日に約2900回までしか試せない。
	LockFor = 5 * time.Minute
	// KnownIPFor は、ログインできた IP を覚えておく期間。
	KnownIPFor = 30 * 24 * time.Hour
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

// ScopeUser と ScopeAdministrator は回数を数える単位。利用者と管理者は別々に数える。
const (
	ScopeUser          = "user"
	ScopeAdministrator = "administrator"
)

func attemptKey(scope, email string) string {
	return scope + ":" + strings.ToLower(strings.TrimSpace(email))
}

func ipKey(account, ip string) string {
	return account + "|" + ip
}

// Forgive はパスワードを再設定できた本人の一時停止を解き、再設定した IP を本人の IP として覚える。
func (g *Guard) Forgive(ctx context.Context, scope, email string) {
	account := attemptKey(scope, email)
	ip := clientip.FromContext(ctx)
	for _, key := range []string{account, ipKey(account, ip)} {
		if err := g.repo.Clear(ctx, key); err != nil {
			logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to clear failures")
		}
	}
	g.rememberIP(ctx, scope, account, ip)
}

func (g *Guard) rememberIP(ctx context.Context, scope, account, ip string) {
	if ip == "" {
		return
	}
	if err := g.repo.RememberIP(ctx, account, ip, KnownIPFor); err != nil {
		logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to remember ip")
	}
}

// isLocked は、この IP からのログインが止められているか。
func (g *Guard) isLocked(ctx context.Context, scope, account, ip string, known bool) bool {
	keys := []string{ipKey(account, ip)}
	if !known {
		keys = append(keys, account)
	}
	for _, key := range keys {
		locked, err := g.repo.IsLocked(ctx, key)
		if err != nil {
			logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to read lock")
			continue
		}
		if locked {
			return true
		}
	}
	return false
}

var dummyPasswordHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("no-such-account"), bcrypt.DefaultCost)
	if err != nil {
		return nil
	}
	return hash
})

// CompareWithoutAccount はアカウントが無いときにも、パスワードを照合したのと同じだけ時間をかける。
// 応答の速さでメールアドレスの登録の有無が分かってしまうのを防ぐ。
func CompareWithoutAccount(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyPasswordHash(), []byte(password))
}

// Attempt は止められていなければ login を実行し、結果に応じて失敗の回数を更新する。
// scope は利用者と管理者で回数を分けるためのもの。IP は clientip.FromContext から読む。
//
// Redis が落ちているときは通す。この仕組みのためにログインを全員止めることはしない。
func Attempt[T any](ctx context.Context, g *Guard, scope, email string, login func() (T, error)) (T, error) {
	var zero T
	account := attemptKey(scope, email)
	ip := clientip.FromContext(ctx)

	known := false
	if ip != "" {
		var err error
		if known, err = g.repo.IsKnownIP(ctx, account, ip); err != nil {
			logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to read known ip")
		}
	}
	if g.isLocked(ctx, scope, account, ip, known) {
		return zero, ErrTooManyAttempts
	}

	result, err := login()
	switch {
	case err == nil:
		// アカウント全体の回数は消さない。本人がログインするたびに、IP を変えながらの
		// 総当たりに試行の枠を与え直すことになるため。
		if err := g.repo.Clear(ctx, ipKey(account, ip)); err != nil {
			logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to clear failures")
		}
		g.rememberIP(ctx, scope, account, ip)
	case errors.Is(err, ErrInvalidCredentials):
		keys := []string{ipKey(account, ip)}
		if !known {
			keys = append(keys, account)
		}
		for _, key := range keys {
			if err := g.repo.RecordFailure(ctx, key, MaxFailures, Window, LockFor); err != nil {
				logger.Log.Warn().Err(err).Str("scope", scope).Msg("login guard: failed to record failure")
			}
		}
	}
	return result, err
}
