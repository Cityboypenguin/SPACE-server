package sse

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// NewHandler は /events 用の Echo ハンドラを返す
func NewHandler(hub *Hub) echo.HandlerFunc {
	return func(c echo.Context) error {
		res := c.Response()
		req := c.Request()

		// SSE 必須ヘッダ
		res.Header().Set(echo.HeaderContentType, "text/event-stream")
		res.Header().Set(echo.HeaderCacheControl, "no-cache")
		res.Header().Set(echo.HeaderConnection, "keep-alive")
		// Nginx 等のバッファリング抑制（環境による）
		res.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := res.Writer.(http.Flusher)
		if !ok {
			return echo.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
		}

		// クライアント登録
		cl := hub.Subscribe()
		defer hub.Unsubscribe(cl)

		// 接続直後に1回送る（任意）
		_ = writeSSE(res.Writer, Event{
			ID:   0,
			Type: "connected",
			Data: map[string]any{"ok": true, "time": time.Now().Format(time.RFC3339Nano)},
			Time: time.Now().Format(time.RFC3339Nano),
		})
		flusher.Flush()

		ctx := req.Context()
		// keep-alive（LB/proxy 対策）
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
				// Publish側で Time を入れていないので、ここで入れる
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
// context を使った認証注入などをしたい場合は、main 側 middleware で
// c.SetRequest(c.Request().WithContext(ctx)) の形で入れてください。
var _ context.Context
