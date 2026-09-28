package middleware

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

// GraphQL への速度制限。1IP あたり毎秒 GraphQLRateLimitRate 本、ためられる
// のは GraphQLRateLimitBurst 本まで。
//
// 定数を外に出してあるのは、Redis 側の置き場（infra/redis.NewRateLimiterStore）が
// 同じ値を使うため。片方だけ変えると、台を増やした瞬間に制限の強さが変わる。
const (
	GraphQLRateLimitRate  = 20.0
	GraphQLRateLimitBurst = 40.0
)

// GraphQLRateLimit は /query への速度制限を返す。
//
// store に nil を渡すとプロセス内のメモリで数える。台が1つなら十分だが、
// 台を増やすと台ごとに別々のバケツを持つので、1IP あたり実質「台数 × 上限」
// まで通る。複数台で動かすときは infra/redis の置き場を渡すこと。
func GraphQLRateLimit(store echoMiddleware.RateLimiterStore) echo.MiddlewareFunc {
	if store == nil {
		store = echoMiddleware.NewRateLimiterMemoryStoreWithConfig(echoMiddleware.RateLimiterMemoryStoreConfig{
			Rate:      GraphQLRateLimitRate,
			Burst:     int(GraphQLRateLimitBurst),
			ExpiresIn: 3 * time.Minute,
		})
	}

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
