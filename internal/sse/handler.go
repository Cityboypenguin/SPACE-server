package sse

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// NewHandler は /events 用の Echo ハンドラを返す。
// 認証は Authorization ヘッダー（JWTAuth middleware 経由）か ?token= クエリパラメータで行う。
// ブラウザ標準の EventSource はカスタムヘッダーを送れないため、?token= を主な認証手段とする。
func NewHandler(hub *Broker, revokedTokenRepo repository.RevokedTokenRepository, userRepo repository.UserRepository) echo.HandlerFunc {
	return func(c echo.Context) error {
		res := c.Response()
		req := c.Request()

		// Auth: middleware が設定したクレームを優先し、なければ ?token= で検証する
		claims, ok := auth.ClaimsFromContext(req.Context())
		if !ok {
			tokenStr := c.QueryParam("token")
			if tokenStr == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing token")
			}
			var err error
			claims, err = auth.ValidateAndVerifyToken(req.Context(), tokenStr, revokedTokenRepo, userRepo)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
			}
		}
		userID := claims.ID

		res.Header().Set(echo.HeaderContentType, "text/event-stream")
		res.Header().Set(echo.HeaderCacheControl, "no-cache")
		res.Header().Set(echo.HeaderConnection, "keep-alive")
		res.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := res.Writer.(http.Flusher)
		if !ok {
			return echo.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
		}

		cl := hub.Subscribe(userID)
		defer hub.Unsubscribe(userID, cl)

		_ = writeSSE(res.Writer, Event{
			ID:   0,
			Type: "connected",
			Data: map[string]any{"ok": true},
			Time: time.Now().Format(time.RFC3339Nano),
		})
		flusher.Flush()

		ctx := req.Context()
		keepAlive := time.NewTicker(15 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-keepAlive.C:
				_, _ = fmt.Fprint(res.Writer, ": ping\n\n")
				flusher.Flush()
			case ev, ok := <-cl.ch:
				if !ok {
					return nil
				}
				if ev.Time == "" {
					ev.Time = time.Now().Format(time.RFC3339Nano)
				}
				if err := writeSSE(res.Writer, ev); err != nil {
					return nil
				}
				flusher.Flush()
			}
		}
	}
}
