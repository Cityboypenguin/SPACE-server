package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// JWTAuth validates Bearer tokens and injects auth claims into request context.
// Requests without a token pass through; protected resolvers must call auth.ClaimsFromContext.
// Requests with an invalid, revoked, or frozen-user token are rejected with 401.
func JWTAuth(revokedTokenRepo repository.RevokedTokenRepository, userRepo repository.UserRepository, pwResetRepo repository.PasswordResetRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return next(c)
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := auth.ValidateAndVerifyToken(c.Request().Context(), tokenStr, revokedTokenRepo, userRepo, pwResetRepo)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
			}

			ctx := auth.WithClaims(c.Request().Context(), claims)
			c.SetRequest(c.Request().WithContext(ctx))

			userID := claims.ID
			go func() {
				_ = userRepo.UpdateLastActiveAt(context.Background(), userID, time.Now().Unix())
			}()

			return next(c)
		}
	}
}
