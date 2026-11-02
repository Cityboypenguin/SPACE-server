// Package ngword は授業内チャットのNGワード。投稿の前に文面を照合して止める
// 検査（Guard）と、管理画面で登録・集計するための操作（AdminService）を持つ。
package ngword

import (
	"context"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// Guard は投稿の文面にNGワードが含まれていないかを検査する。
type Guard interface {
	// Screen は texts のどれかがNGワードに当たれば、止めた記録を残して InvalidInput を返す。
	// roomID は分かるときだけ渡す（nil でよい）。
	Screen(ctx context.Context, userID int64, roomID *int64, source model.NGWordSource, texts ...string) error
}

// ScreenFor は claims の本人の投稿を検査する。管理者の投稿は止めない。
// g が nil なら何もしない（NGワードを使わない構成・テスト）。
func ScreenFor(ctx context.Context, g Guard, claims *auth.Claims, roomID *int64, source model.NGWordSource, texts ...string) error {
	if g == nil || claims == nil || authz.IsAdminRole(claims.Role) {
		return nil
	}
	return g.Screen(ctx, claims.ID, roomID, source, texts...)
}

// NoopGuard は何も止めない（NGワードを使わない構成・テスト用）。
type NoopGuard struct{}

func (NoopGuard) Screen(context.Context, int64, *int64, model.NGWordSource, ...string) error {
	return nil
}

// BlockedMessage は止めたときにユーザーへ返す文言。どの語に当たったかは明かさない
// （明かすと、すり抜ける言い換えを探す手がかりになる）。
const BlockedMessage = "使えない言葉が含まれているため、投稿できません。内容を直してください"

// cacheTTL は登録内容を覚えておく長さ。投稿のたびにDBを引かないための保持で、
// 管理画面での変更はこの手元の保持を捨てて即座に効く（複数台でも、この秒数のうちに揃う）。
const cacheTTL = 15 * time.Second

type guard struct {
	repo repository.NGWordRepository
	now  func() time.Time

	mu       sync.Mutex
	matcher  *model.NGWordMatcher
	loadedAt time.Time
	// version は Invalidate のたびに増える。読み込みの間に登録が変わったら、その結果を覚えない。
	version uint64
}

// Invalidator は登録内容が変わったことを知らせる口（AdminService が使う）。
type Invalidator interface {
	Invalidate()
}

type GuardWithInvalidate interface {
	Guard
	Invalidator
}

func NewGuard(repo repository.NGWordRepository) GuardWithInvalidate {
	return &guard{repo: repo, now: time.Now}
}

func (g *guard) Invalidate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.matcher = nil
	g.version++
}

// current は照合器を返す。DB を読む間はロックを持たない（DB が遅いときに、授業内チャットの
// 投稿が検査の順番待ちで止まらないようにするため）。期限切れの瞬間に同時に来た投稿は
// それぞれ読むが、数秒に一度のことなので許容する。
func (g *guard) current(ctx context.Context) (*model.NGWordMatcher, error) {
	g.mu.Lock()
	if g.matcher != nil && g.now().Sub(g.loadedAt) < cacheTTL {
		defer g.mu.Unlock()
		return g.matcher, nil
	}
	version := g.version
	g.mu.Unlock()

	words, err := g.repo.ListNGWords(ctx)

	g.mu.Lock()
	defer g.mu.Unlock()
	if err != nil {
		// 期限が切れた内容でも、検査しないよりは良いので、残っていれば使う。
		if g.matcher != nil {
			logger.Log.Error().Err(err).Str("component", "ngword").Msg("failed to reload ng words; using the previous list")
			return g.matcher, nil
		}
		return nil, err
	}
	matcher := model.NewNGWordMatcher(words)
	if version == g.version {
		g.matcher = matcher
		g.loadedAt = g.now()
	}
	return matcher, nil
}

func (g *guard) Screen(ctx context.Context, userID int64, roomID *int64, source model.NGWordSource, texts ...string) error {
	m, err := g.current(ctx)
	if err != nil {
		// 検査のための読み取りが失敗しても、投稿そのものは止めない（NGワードの不具合で
		// 授業内チャット全体が書けなくなるほうが害が大きい）。
		logger.Log.Error().Err(err).Str("component", "ngword").Msg("failed to load ng words; skipping the check")
		return nil
	}
	for _, t := range texts {
		hit := m.Match(t)
		if hit == nil {
			continue
		}
		rec := &model.NGWordHit{
			NGWordID:  &hit.ID,
			Pattern:   hit.Pattern,
			UserID:    userID,
			RoomID:    roomID,
			Source:    source,
			CreatedAt: g.now(),
		}
		if err := g.repo.RecordNGWordHit(ctx, rec); err != nil {
			// 記録に失敗しても、止めること自体は変えない。
			logger.Log.Error().Err(err).Str("component", "ngword").Msg("failed to record an ng word hit")
		}
		return apperr.InvalidInput(BlockedMessage)
	}
	return nil
}
