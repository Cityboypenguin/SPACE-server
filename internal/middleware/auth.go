package middleware

import (
	"net/http"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// JWTAuth validates Bearer tokens and injects auth claims into request context.
// Requests without a token pass through; protected resolvers must call auth.ClaimsFromContext.
// Requests with an invalid, revoked, or frozen-user token are rejected with 401.
//
// activity は認証できたリクエストの活動記録（最終アクセス時刻・活動日）。
// nil なら記録しない（記録先を配線しない起動経路向け）。毎リクエスト書きに行くのは
// activity 側が間引く（internal/middleware/user_activity.go 参照）。
func JWTAuth(revokedTokenRepo repository.RevokedTokenRepository, userRepo repository.UserRepository, adminRepo repository.AdministratorRepository, activity *UserActivityRecorder) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return next(c)
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := auth.ValidateAndVerifyToken(c.Request().Context(), tokenStr, revokedTokenRepo, userRepo, adminRepo)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
			}

			// トークン文字列も載せる。長寿命の接続（WebSocket / SSE）は、
			// このリクエストで受け取ったトークンを握ったまま何時間も生きるので、
			// 接続中に確かめ直すための元手が要る（auth.WithToken のコメント参照）。
			ctx := auth.WithClaims(c.Request().Context(), claims)
			ctx = auth.WithToken(ctx, tokenStr)
			c.SetRequest(c.Request().WithContext(ctx))

			if activity != nil {
				activity.Record(ctx, claims.ID)
			}

			return next(c)
		}
	}
}
