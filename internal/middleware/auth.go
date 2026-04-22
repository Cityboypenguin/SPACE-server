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
// Requests with an invalid or revoked token are rejected with 401.
func JWTAuth(revokedTokenRepo repository.RevokedTokenRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return next(c)
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := auth.ValidateToken(tokenStr)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			revoked, err := revokedTokenRepo.IsRevoked(c.Request().Context(), tokenStr)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to verify token")
			}
			if revoked {
				return echo.NewHTTPError(http.StatusUnauthorized, "token has been revoked")
			}

			ctx := auth.WithClaims(c.Request().Context(), claims)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
