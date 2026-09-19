// internal/middleware/block_filter.go
package middleware

import (
	"context"
	"errors"
	"sync"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

type blockListKey struct{}

// blockList はリクエスト1本ぶんのブロック一覧と、「そもそも読めたか」を持つ。
//
// err を別に持つのが要点。以前は取得できたIDだけを Context に入れていたので、
// 「ブロック相手が1人も居ない」と「DBエラーで読めなかった」が後段から同じに
// 見えていた。ブロックは「見たくない相手が見えない」ための機能なので、
// 読めなかったときに素通しするのは機能の否定にあたる（フェイルオープン）。
type blockList struct {
	once   sync.Once
	repo   repository.BlockerRepository
	userID int64
	ids    []int64
	err    error
}

func (b *blockList) load(ctx context.Context) {
	b.once.Do(func() {
		b.ids, b.err = b.repo.GetBlockedAndBlockerIDs(ctx, b.userID)
		if b.err != nil {
			logger.Log.Error().Err(b.err).
				Str("component", "block_filter").
				Int64("user_id", b.userID).
				Msg("failed to load block list; block-filtered queries will fail closed")
		}
	})
}

// ErrBlockListUnavailable は、このリクエストのブロック一覧を取得できなかったこと。
//
// ブロック除外を含むクエリは、これを見たら結果を返さずに失敗する（後段で
// 素通しさせないため）。呼び出し側が種類で分岐できるよう、文字列一致ではなく
// この値を errors.Is で見られる形にしてある。
var ErrBlockListUnavailable = errors.New("block list is unavailable for this request")

// BlockFilter は遅延ロード用のブロック一覧を Context に付与する。
func BlockFilter(blockRepo repository.BlockerRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// 直前の JWTAuth ミドルウェアで設定されたユーザー情報を Context から取得
			if claims, ok := auth.ClaimsFromContext(ctx); ok && claims.ID != 0 {
				// DB 取得は AppendBlockFilter が必要とした時点まで遅らせる。同じ
				// リクエスト内で複数クエリが使っても sync.Once により1回だけ読む。
				ctx = context.WithValue(ctx, blockListKey{}, &blockList{repo: blockRepo, userID: claims.ID})
				// 更新した Context を Request に再セットして後続へ渡す
				c.SetRequest(c.Request().WithContext(ctx))
			}

			return next(c)
		}
	}
}

// BlockListFromContext は、リポジトリ層でブロックリストを取り出すためのヘルパーです。
//
// 戻りのエラーが非 nil なら、このリクエストではブロック除外を適用できない。
// 呼び出し側はそのまま返して失敗させること（空扱いで続行しないこと）。
//
// 未認証リクエストでは (nil, nil) を返す。除外すべき相手が居ないだけで、
// 取得に失敗したわけではないため。
func BlockListFromContext(ctx context.Context) ([]int64, error) {
	list, ok := ctx.Value(blockListKey{}).(*blockList)
	if !ok {
		return nil, nil
	}
	list.load(ctx)
	if list.err != nil {
		return nil, ErrBlockListUnavailable
	}
	return list.ids, nil
}
