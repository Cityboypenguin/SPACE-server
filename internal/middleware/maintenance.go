package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/labstack/echo/v4"
)

// adminBypassOps are GraphQL operations that must work during maintenance so
// administrators can log in even when they have no active session.
var adminBypassOps = []string{"loginAdministrator", "refreshAdministratorToken"}

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

			if isAdminBypassRequest(c) {
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

// isAdminBypassRequest returns true when the POST body contains an admin-only
// GraphQL operation that must remain accessible during maintenance (login /
// token refresh). The body is buffered and restored so downstream handlers
// can still read it.
func isAdminBypassRequest(c echo.Context) bool {
	if c.Request().Method != http.MethodPost {
		return false
	}
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return false
	}
	c.Request().Body = io.NopCloser(bytes.NewReader(body))

	bodyStr := string(body)
	for _, op := range adminBypassOps {
		if strings.Contains(bodyStr, op) {
			return true
		}
	}
	return false
}
