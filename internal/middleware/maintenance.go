package middleware

import (
	"net/http"
	"sync/atomic"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/labstack/echo/v4"
)

// MaintenanceMode blocks non-admin requests with 503 when the maintenance flag is set.
// Must be registered after JWTAuth so that admin claims are already in context.
func MaintenanceMode(flag *atomic.Bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !flag.Load() {
				return next(c)
			}

			claims, ok := auth.ClaimsFromContext(c.Request().Context())
			if ok && maintenanceIsAdmin(claims.Role) {
				return next(c)
			}

			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"message": "server_maintenance",
			})
		}
	}
}

func maintenanceIsAdmin(role string) bool {
	return role == "admin" || role == "administrator"
}
