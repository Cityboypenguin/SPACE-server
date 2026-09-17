package chat

import (
	"context"
	"sync"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

// AsyncRunner は配信の一部をリクエストの応答時間の外へ出すための小さな実行口。
//
// # なぜ「一部」なのか（全部を非同期にしない）
//
// 配信は EventPublisher のコメントどおり「保存が済んだあとのベストエフォートな
// 周辺作用」だが、その中で扱いが2つに割れる。
//
//   - 順序が要るもの: 購読中の画面へ流す PubSub（message:added / updated / deleted）。
//     インメモリの publish なので安価な一方、順序が入れ替わるとチャット本体の
//     メッセージが入れ替わって表示される。これはリクエストの中で同期に実行する
//     （graph/chat_events.go 参照）。プレビューが一瞬巻き戻る程度の話ではなく、
//     会話そのものが崩れて見えるので、速さと引き換えにしてよい部分ではない。
//   - 順序が要らないもの: room_changed の SSE 配信と通知（DM・返信・メンション）。
//     宛先ぶんのループや通知行の書き込みが入り、宛先が多いほど時間がかかる。
//     room_changed は「このルームが更新された」という事実だけを運び、受け取った
//     クライアントは messageID の新旧を見て古いものを捨てる（そもそも一覧を
//     取り直すだけ）ので、入れ替わっても壊れない。ここをこの Runner へ渡す。
//
// # 以前の実装との違い
//
// 以前はルーム単位のFIFOキューとワーカー、積み残し上限を持つ decorator だった。
// 順序を保つ機構が要ったのは、送信のたびに走る未読集計
// （履修者×メッセージの JOIN）を非同期へ逃がすために配信を丸ごと外へ出しており、
// その中に順序の要る PubSub も混ざっていたから。集計そのものをやめ、順序の要る
// PubSub を同期に戻したので、キューで直列化する理由が無くなった。
// 残しているのは context の扱い・panic の握り（下記）・停止時の待ち合わせだけ。
//
// # 順序について
//
// この Runner は順序を保証しない。渡した関数は goroutine で即座に走り、
// 同じルームの2件がどちらの順で完了するかは決まらない。順序が要るものを
// ここへ渡さないこと。
//
// # 値の共有について
//
// 渡す関数がイベントの *model.Room / *model.Message を掴んだままになる。
// 呼び出し元（usecase/chat の各サービス）は保存後にこれらを書き換えないので
// 読み取りが並行するだけだが、渡したあとに中身を触る呼び出し元を増やさないこと
// （触るならコピーを渡すこと）。
type AsyncRunner struct {
	// wg は走っている配信の数。停止時の待ち合わせとテストの同期に使う。
	wg sync.WaitGroup
}

// NewAsyncRunner は配信をリクエストの外で走らせる Runner を返す。
func NewAsyncRunner() *AsyncRunner { return &AsyncRunner{} }

// Go は fn をリクエストの外で1回実行する。name は失敗ログの識別子。
//
// ctx から切り離すのは「キャンセルだけ」（context.WithoutCancel）。リクエストの ctx を
// そのまま goroutine へ渡すと、レスポンスを返した時点でキャンセルされ、配信中の
// DB アクセス（通知の保存、授業ルームの宛先の取得）が context canceled で軒並み
// 失敗する。かといって context.Background() に差し替えると ctx に載っている値まで
// 落ちてしまい、認証情報（internal/auth の Claims。監査ログや権限判定が読む）や
// ロガー・リクエストIDが消える。値は保ちキャンセルだけ外す WithoutCancel が、
// ちょうど要るものになる。
//
// タイムアウトを別に張らないのは、配信に上限時間を付けると途中で切られて
// 「通知だけ出ていない」状態が起きるため。残った配信は Wait で回収する。
func (r *AsyncRunner) Go(ctx context.Context, name string, fn func(context.Context)) {
	if fn == nil {
		return
	}
	detached := context.WithoutCancel(ctx)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() {
			// goroutine の panic はプロセスごと落とす。同期実行だったころは
			// GraphQL のリカバリまで上がってリクエストがエラーになるだけで済んだ。
			// 配信の取りこぼしでサーバを落とすのは割に合わないので必ず recover し、
			// 握り潰した事実が消えないようログには必ず出す。
			if p := recover(); p != nil {
				logger.Log.Error().
					Str("component", "chat_event_publisher").
					Str("delivery", "async_panic").
					Str("event", name).
					Interface("panic", p).
					Msg("recovered from a panic while publishing a chat event")
			}
		}()
		fn(detached)
	}()
}

// Wait は走っている配信が終わるまで待つ。
//
// 用途は2つ。(1) 停止時に、レスポンスを返し終えたあとの取りこぼしを DB 接続を
// 閉じる前に減らす。(2) テストで「非同期だから確かめられない」を避ける。
// ctx が先に切れたら待つのをやめて ctx.Err() を返す（処理中のものは走り続ける）。
func (r *AsyncRunner) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
