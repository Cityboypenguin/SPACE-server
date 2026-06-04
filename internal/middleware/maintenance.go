package middleware

import (
	"net/http"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// MaintenanceGuard はメンテナンス中に一般ユーザーのリクエストを 503 で遮断するミドルウェアです。
// 管理者（role: admin / administrator）および未認証リクエストの認証チェック自体は通過させます。
// ただし /query エンドポイントへのリクエストのうち、管理者以外は全て 503 を返します。
func MaintenanceGuard(maintenanceRepo repository.MaintenanceRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// /query 以外（ヘルスチェック等）は対象外
			if c.Path() != "/query" {
				return next(c)
			}

			ctx := c.Request().Context()

			active, err := maintenanceRepo.IsMaintenance(ctx)
			if err != nil || !active {
				return next(c)
			}

			// 管理者は通過
			if claims, ok := auth.ClaimsFromContext(ctx); ok {
				role := claims.Role
				if role == "admin" || role == "administrator" {
					return next(c)
				}
			}

			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"message": "現在メンテナンス中です。しばらくお待ちください。",
			})
		}
	}
}
