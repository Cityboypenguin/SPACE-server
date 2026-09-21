// Package ssehttp は /events（通知の SSE）の HTTP 入口。
//
// internal/sse から切り出してある。あちらは「誰に何を配るか」を持つ中核で、
// Echo も HTTP も知らなくてよい。混ぜておくと、配信の仕組みを試すのに
// HTTP の道具立てが要るし、別の入口（別のフレームワーク・テスト用の直結）を
// 足すときに中核ごと引きずることになる。
package ssehttp

import (
	"context"
	"fmt"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"net/http"
	"strconv"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// NewHandler は /events 用の Echo ハンドラを返す。
//
// # 認証方式（2026-09 変更）
//
// 認証は次の2経路。上から順に試す。
//
//  1. Authorization ヘッダー（JWTAuth middleware がクレームを ctx に載せている）。
//     fetch/EventSource ポリフィルなど、ヘッダーを付けられるクライアント向け。
//  2. ?ticket= — 使い捨ての短命チケット（issueNotificationStreamTicket で発行）。
//     ブラウザ標準の EventSource はカスタムヘッダーを送れないため、これが本命。
//
// # なぜクッキーではなくチケットにしたか
//
// EventSource がヘッダーを送れない以上、選択肢はクッキーか URL 上のチケットだった。
// クッキーを選ばなかったのは、このアプリが**どこでも認証クッキーを使っていない**ため:
// アクセストークンもリフレッシュトークンもクライアントの localStorage にあり、
// Authorization ヘッダーで送られる（SPACE-client の lib/authStorage.ts）。
// ここだけクッキーを導入すると、
//
//   - ログイン・リフレッシュ・ログアウトの全経路（一般ユーザーと管理者の2系統）に
//     Set-Cookie / 失効処理を足すことになる
//   - クッキーは自動で付くので、/events に対する CSRF と SameSite の検討が要る
//   - 開発はプロキシで同一オリジンだが、本番のオリジン構成に依存する設定が増える
//
// と、たった1つのエンドポイントのために認証基盤全体へ手が入る。
// チケットなら追加されるのは「発行するミューテーション1つ」と「引き換え」だけで、
// 既存の認証経路には触らない。URL に残る点は同じだが、チケットは**1回使ったら無効・
// 30秒で失効**なので、アクセスログから拾っても再利用できない（JWT は有効期限まで
// そのまま使えてしまう。これが元の指摘そのもの）。
//
// # 接続したあとも確かめ直す
//
// 引き換えは接続の入口で1度きり。ここで終わりにすると、繋がっている間ずっと
// そのときの権限で動き続ける（凍結・退会・ログアウト・パスワード変更のどれも
// 効かない）。watch に渡した関数が定期的に認証をやり直し、通らなくなったら
// 接続の ctx を終わらせる。

// WatchSessionFunc は接続中に認証をやり直す口（本番では auth.WatchSession を束ねたもの）。
//
// internal/sse が認証の置き場（失効リスト・利用者表・管理者表）を直接知らずに
// 済むよう、関数1つで受け取る。nil なら確かめ直さない（テストや、認証を
// 配線しない起動経路向け）。
type WatchSessionFunc func(ctx context.Context, token string) context.Context

func NewHandler(hub *sse.Broker, ticketRepo repository.SSETicketRepository, watch WatchSessionFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		res := c.Response()
		req := c.Request()

		session, err := authenticate(c, ticketRepo)
		if err != nil {
			return err
		}
		userID := session.UserID

		res.Header().Set(echo.HeaderContentType, "text/event-stream")
		res.Header().Set(echo.HeaderCacheControl, "no-cache")
		res.Header().Set(echo.HeaderConnection, "keep-alive")
		res.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := res.Writer.(http.Flusher)
		if !ok {
			return echo.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
		}

		// Last-sse.Event-ID が送られていれば再接続とみなしてリプレイする
		// ヘッダーがなければ初回接続（lastEventID = -1 でリプレイなし）
		lastEventID := -1
		if raw := req.Header.Get("Last-sse.Event-ID"); raw != "" {
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

		_ = writeSSE(res.Writer, sse.Event{
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

		// 接続時にベルの未読数を数えて送るのはやめた。
		//
		// 以前はここで接続・再接続のたびに COUNT を撃ち、失敗したら 0 を送っていた。
		// 0 は「未読が無い」という嘘で、DB が不調なときほどベルが静かになる
		// （＝一番気づいてほしいときに気づけない）。
		//
		// 数はサーバから配らず、クライアントが接続できた時点で
		// myUnreadNotificationCount を取りに行く（room_changed で未読数を配るのを
		// やめたのと同じ方針）。取得が失敗すればクライアント側で失敗として扱えるので、
		// 嘘の 0 が画面に出ることも無い。
		flusher.Flush()

		// 接続中も認証をやり直す。ここを足さないと、チケットを引き換えた
		// 一瞬の権限のまま何時間でも繋がり続ける（凍結・退会・ログアウト・
		// パスワード変更のどれも効かない）。通らなくなれば ctx が終わり、
		// 下のループが抜けて接続が閉じる。
		ctx := req.Context()
		if watch != nil && session.Token != "" {
			ctx = watch(ctx, session.Token)
		}
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
			case ev, ok := <-cl.Events():
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

// authenticate は /events の接続要求から、この接続の認証の素を取り出す。
// 経路の優先順位と、それぞれを採る理由は NewHandler のコメントを参照。
func authenticate(
	c echo.Context,
	ticketRepo repository.SSETicketRepository,
) (repository.StreamSession, error) {
	req := c.Request()

	// 1. middleware が Authorization ヘッダーを検証済みならそれを使う。
	if claims, ok := auth.ClaimsFromContext(req.Context()); ok {
		// トークンは接続中の確かめ直しに使う。取れなければ確かめ直しが効かない
		// だけで、接続自体は張れる（middleware が検証済みなので）。
		token, _ := auth.TokenFromContext(req.Context())
		return repository.StreamSession{UserID: claims.ID, Token: token}, nil
	}

	// 2. 使い捨てチケット。引き換えは1回きり（Consume が取得と削除を不可分に行う）。
	if ticket := c.QueryParam("ticket"); ticket != "" {
		if ticketRepo == nil {
			return repository.StreamSession{}, echo.NewHTTPError(http.StatusInternalServerError, "ticket auth unavailable")
		}
		session, ok, err := ticketRepo.Consume(req.Context(), ticket)
		if err != nil {
			// Redis が落ちている等。「無効なチケット」と区別できないと調査で困るので
			// ログには残すが、クライアントへは理由を返さない（チケットの有無を
			// 探る手がかりにさせない）。
			logger.Log.Error().Err(err).Str("component", "sse").Msg("failed to consume sse ticket")
			return repository.StreamSession{}, echo.NewHTTPError(http.StatusInternalServerError, "failed to verify ticket")
		}
		if !ok {
			// 期限切れ・使用済み・でたらめ、のどれかを区別しない。
			// 区別して返すと、チケットの推測に使える情報になる。
			return repository.StreamSession{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired ticket")
		}
		return session, nil
	}

	return repository.StreamSession{}, echo.NewHTTPError(http.StatusUnauthorized, "missing ticket")
}
