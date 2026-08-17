package sse

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// NewHandler は /events 用の Echo ハンドラを返す。
// 認証は Authorization ヘッダー（JWTAuth middleware 経由）か ?token= クエリパラメータで行う。
// ブラウザ標準の EventSource はカスタムヘッダーを送れないため、?token= を主な認証手段とする。
func NewHandler(hub *Broker, notifRepo repository.NotificationRepository, revokedTokenRepo repository.RevokedTokenRepository, userRepo repository.UserRepository, pwResetRepo repository.PasswordResetRepository) echo.HandlerFunc {
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
			claims, err = auth.ValidateAndVerifyToken(req.Context(), tokenStr, revokedTokenRepo, userRepo, pwResetRepo)
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

		// Last-Event-ID が送られていれば再接続とみなしてリプレイする
		// ヘッダーがなければ初回接続（lastEventID = -1 でリプレイなし）
		lastEventID := -1
		if raw := req.Header.Get("Last-Event-ID"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				lastEventID = n
			}
		}

		cl, missed, err := hub.Subscribe(userID, lastEventID)
		if err != nil {
			return echo.NewHTTPError(http.StatusTooManyRequests, "too many SSE connections")
		}
		defer hub.Unsubscribe(userID, cl)

		now := time.Now().Format(time.RFC3339Nano)

		_ = writeSSE(res.Writer, Event{
			ID:   0,
			Type: "connected",
			Data: map[string]any{"ok": true},
			Time: now,
		})

		// 切断中に積まれた未配信イベントをリプレイ
		// replayed=true を付けることでクライアント側でトーストをスキップさせる
		for _, ev := range missed {
			replayEv := ev
			copied := make(map[string]any, len(ev.Data)+1)
			for k, v := range ev.Data {
				copied[k] = v
			}
			copied["replayed"] = true
			replayEv.Data = copied
			if replayEv.Time == "" {
				replayEv.Time = now
			}
			_ = writeSSE(res.Writer, replayEv)
		}

		// 現在の未読数を送信（再接続時にバッジ数を正確に同期する）
		unreadCount, _ := notifRepo.CountUnread(req.Context(), userID)
		_ = writeSSE(res.Writer, Event{
			Type: "sync",
			Data: map[string]any{"unreadCount": unreadCount},
			Time: now,
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
				// ping の書き込みが失敗したらクライアントは既に居ないとみなし、
				// goroutine・チャンネル・接続スロットを最大 15 秒で回収する。
				if _, err := fmt.Fprint(res.Writer, ": ping\n\n"); err != nil {
					return nil
				}
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
