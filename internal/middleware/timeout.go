package middleware

import (
	"time"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

// RequestTimeout adds a context deadline to all HTTP requests except WebSocket upgrades
// and SSE connections, which are long-lived and must not be cancelled by a timeout.
func RequestTimeout(d time.Duration) echo.MiddlewareFunc {
	return echoMiddleware.TimeoutWithConfig(echoMiddleware.TimeoutConfig{
		Timeout: d,
		Skipper: func(c echo.Context) bool {
			return c.Request().Header.Get("Upgrade") == "websocket" || c.Path() == "/events"
		},
	})
}
