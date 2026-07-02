package middleware

import (
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/metrics"
	"github.com/labstack/echo/v4"
)

func MetricsMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			durationMs := float64(time.Since(start).Microseconds()) / 1000.0
			isError := c.Response().Status >= 500
			if err != nil {
				isError = true
			}
			metrics.Global.RecordRequest(durationMs, isError)
			return err
		}
	}
}
