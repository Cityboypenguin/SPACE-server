package sse

import (
	"context"
	"encoding/json" // 追加: JSON変換用
	"fmt"
	"io" // 追加: io.Writer用
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// NewSSEHandler は /events 用の Echo ハンドラを返す
// 修正: "NewHandler" が重複していたため "NewSSEHandler" に変更しました
func NewSSEHandler(hub *Hub) echo.HandlerFunc {
	return func(c echo.Context) error {
		res := c.Response()
		req := c.Request()

		// SSE 必須ヘッダ
		res.Header().Set(echo.HeaderContentType, "text/event-stream")
		res.Header().Set(echo.HeaderCacheControl, "no-cache")
		res.Header().Set(echo.HeaderConnection, "keep-alive")
		// Nginx 等のバッファリング抑制
		res.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := res.Writer.(http.Flusher)
		if !ok {
			return echo.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
		}

		// クライアント登録
		cl := hub.Subscribe()
		defer hub.Unsubscribe(cl)

		// 接続直後に1回送る（任意）
		// エラーハンドリングを追加しましたが、接続直後の失敗はログ出力程度で良い場合が多いです
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
				// コメント: keep-alive用のコメント行を送る
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
					// 書き込みエラー（クライアント切断など）の場合はループを抜ける
					return nil
				}
				flusher.Flush()
			}
		}
	}
}

// writeSSE はSSEのフォーマットに従ってデータを書き込みます
// 実装されていなかった部分です
func writeSSE(w io.Writer, ev Event) error {
	// IDの書き込み
	if ev.ID != 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", ev.ID); err != nil {
			return err
		}
	}

	// Event Typeの書き込み
	if ev.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", ev.Type); err != nil {
			return err
		}
	}

	// Dataの書き込み (JSONマーシャル)
	if ev.Data != nil {
		jsonData, err := json.Marshal(ev.Data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n", jsonData); err != nil {
			return err
		}
	}

	// メッセージの終わり（空行）
	_, err := fmt.Fprint(w, "\n")
	return err
}

var _ context.Context
