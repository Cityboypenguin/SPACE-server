package middleware

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

// GraphQLRateLimit はIPベースで/queryへのリクエストを制限する。
// 開発・テスト環境ではE2Eテストの並列実行が同一IPからバーストするため、本番環境でのみ有効化する。
func GraphQLRateLimit(isProd bool) echo.MiddlewareFunc {
	if !isProd {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		}
	}

	store := echoMiddleware.NewRateLimiterMemoryStoreWithConfig(echoMiddleware.RateLimiterMemoryStoreConfig{
		Rate:      20,
		Burst:     40,
		ExpiresIn: 3 * time.Minute,
	})

	return echoMiddleware.RateLimiterWithConfig(echoMiddleware.RateLimiterConfig{
		Skipper: func(c echo.Context) bool {
			return c.Path() != "/query"
		},
		Store: store,
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid rate limit identifier")
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests")
		},
	})
}
