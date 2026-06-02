// internal/middleware/block_filter.go
package middleware

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

type blockListKey struct{}

// BlockFilter は、ログインユーザーのブロック/被ブロックID一覧を取得しContextに付与するEchoミドルウェアです
func BlockFilter(blockRepo repository.BlockerRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// 直前の JWTAuth ミドルウェアで設定されたユーザー情報を Context から取得
			if claims, ok := auth.ClaimsFromContext(ctx); ok && claims.ID != 0 {
				// DBから自分と相手のブロック関係にある全IDを取得
				blockedIDs, err := blockRepo.GetBlockedAndBlockerIDs(ctx, claims.ID)

				// エラーがなく、かつブロック関係のユーザーが1人以上いる場合のみContextに詰める
				if err == nil && len(blockedIDs) > 0 {
					ctx = context.WithValue(ctx, blockListKey{}, blockedIDs)
					// 更新した Context を Request に再セットして後続へ渡す
					c.SetRequest(c.Request().WithContext(ctx))
				}
			}

			return next(c)
		}
	}
}

// GetBlockListFromContext は、リポジトリ層でブロックリストを取り出すためのヘルパーです
func GetBlockListFromContext(ctx context.Context) []int64 {
	ids, ok := ctx.Value(blockListKey{}).([]int64)
	if !ok {
		return nil
	}
	return ids
}
