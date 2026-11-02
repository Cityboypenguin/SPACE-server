package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// GraphQL への速度制限は2段ある。
//
// GraphQLIPRateLimit は JWT 検証（DB/Redis照合あり）より前に置く粗い制限。学内の Wi-Fi のように
// 大勢が同じ IP を共有するので、大きめの値にして、1つの IP からの洪水だけを止める。
// GraphQLRateLimit は JWT 検証のあとに置く制限で、ログイン中は利用者ごと、未ログインは IP ごとに
// 数える。ログインの総当たりなどを抑えるのはこちら。
//
// 定数を外に出してあるのは、Redis 側の置き場（infra/redis.NewRateLimiterStore）が
// 同じ値を使うため。片方だけ変えると、台を増やした瞬間に制限の強さが変わる。
const (
	GraphQLRateLimitRate    = 20.0
	GraphQLRateLimitBurst   = 40.0
	GraphQLIPRateLimitRate  = 200.0
	GraphQLIPRateLimitBurst = 400.0
)

// GraphQLIPRateLimit は /query への IP ごとの粗い制限を返す。
func GraphQLIPRateLimit(store echoMiddleware.RateLimiterStore) echo.MiddlewareFunc {
	return newGraphQLRateLimit(store, GraphQLIPRateLimitRate, GraphQLIPRateLimitBurst, func(c echo.Context) string {
		return "ip:" + c.RealIP()
	})
}

// GraphQLRateLimit は /query への利用者ごとの制限を返す。未ログインは IP ごとに数える。
// JWTAuth より後に置くこと（前に置くと全員が未ログイン扱いになる）。
func GraphQLRateLimit(store echoMiddleware.RateLimiterStore) echo.MiddlewareFunc {
	return newGraphQLRateLimit(store, GraphQLRateLimitRate, GraphQLRateLimitBurst, func(c echo.Context) string {
		if claims, ok := auth.ClaimsFromContext(c.Request().Context()); ok && claims.ID != 0 {
			return fmt.Sprintf("user:%s:%d", claims.Role, claims.ID)
		}
		return "anon:" + c.RealIP()
	})
}

// newGraphQLRateLimit は store に nil を渡すとプロセス内のメモリで数える。台が1つなら十分だが、
// 台を増やすと台ごとに別々のバケツを持つので、実質「台数 × 上限」まで通る。
// 複数台で動かすときは infra/redis の置き場を渡すこと。
func newGraphQLRateLimit(store echoMiddleware.RateLimiterStore, perSecond, burst float64, identify func(echo.Context) string) echo.MiddlewareFunc {
	if store == nil {
		store = echoMiddleware.NewRateLimiterMemoryStoreWithConfig(echoMiddleware.RateLimiterMemoryStoreConfig{
			Rate:      rate.Limit(perSecond),
			Burst:     int(burst),
			ExpiresIn: 3 * time.Minute,
		})
	}

	return echoMiddleware.RateLimiterWithConfig(echoMiddleware.RateLimiterConfig{
		Skipper: func(c echo.Context) bool {
			return c.Path() != "/query"
		},
		Store: store,
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return identify(c), nil
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid rate limit identifier")
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many requests")
		},
	})
}
