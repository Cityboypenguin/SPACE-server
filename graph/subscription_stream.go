package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/rs/zerolog"
)

// subscriptionSource は購読に使う PubSub の狭い口。
//
// *pubsub.PubSub をそのまま受け取らないのは、chatEventPubSub（graph/chat_events.go）
// と同じ理由。転送ループだけを単体で確かめられるようにするため、実際に呼ぶ
// メソッドだけを切っている（*pubsub.PubSub はそのまま満たすので配線は変わらない）。
type subscriptionSource interface {
	Subscribe(topic string) chan interface{}
	Unsubscribe(topic string, ch chan interface{})
}

// subscriptionScope は購読のログに載せる「誰が・どこを」。
//
// RoomID は空なら載せない（質問・投票・管理画面の購読のように、ルームを
// 特定できない／持たない購読があるため）。topic は転送ループ側が持っているので
// ここには入れない。
type subscriptionScope struct {
	UserID int64
	RoomID string
}

// subscribeTopic は PubSub の1トピックを購読し、型 T の値だけを GraphQL の
// subscription チャンネルへ流す。
//
// 以前は message / question / answer / poll / read_status / import の6か所へ
// 同じループを写経していた（型と、ごく一部の絞り込みだけが違う）。写経が増えると
// 「context 終了時に close する」「終わったら必ず Unsubscribe する」といった
// 取りこぼすと goroutine が残る部分が、直すたびに片方だけ直る。ループと後始末は
// ここ1本に集め、認証・権限判定・トピック名の組み立てという購読ごとに違う部分は
// 呼び出し側に残す。
//
// 型が合わない値を捨てるのは従来どおり（1つのトピックに別の型が流れてくるのは
// 配信側のバグだが、購読側を落とす理由にはならない）。
func subscribeTopic[T any](ctx context.Context, src subscriptionSource, topic string, scope subscriptionScope) <-chan T {
	return subscribeTopicFunc(ctx, src, topic, scope, func(v T) (T, bool) { return v, true })
}

// subscribeTopicFunc は subscribeTopic に「流す前の変換・絞り込み」を足した版。
//
// convert が false を返した値は配信しない。用途は2つだけ:
//   - 既読の購読で自分自身のイベントを落とす（絞り込み）
//   - 取り込み状況の購読でドメインの値を GraphQL 型へ直す（変換）
//
// 変換をループの中に持たせているのは、変換の失敗や絞り込みで「1件送らない」時に
// ループを止めてはいけないため（呼び出し側で map しようとすると、そのたびに
// もう1本ループを書くことになる）。
func subscribeTopicFunc[S any, T any](
	ctx context.Context,
	src subscriptionSource,
	topic string,
	scope subscriptionScope,
	convert func(S) (T, bool),
) <-chan T {
	logSubscription(scope, topic).Msg("subscription start")

	// バッファ1は従来どおり。gqlgen 側の読み出しが遅れても配信元
	// （pubsub.Publish）を止めないための1段で、詰まればここでブロックする。
	out := make(chan T, 1)
	sub := src.Subscribe(topic)

	go func() {
		// 後始末は defer に寄せる。終了地点が「受信待ちでの ctx 終了」「PubSub の
		// クローズ」「送信待ちでの ctx 終了」の3つに増えたので、各 return の手前で
		// close(out) を書き並べると閉じ忘れが必ず出る。LIFO なので close(out) →
		// Unsubscribe の順で走る。
		defer src.Unsubscribe(topic, sub)
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				logSubscription(scope, topic).Str("reason", "context_done").Msg("subscription end")
				return
			case data, ok := <-sub:
				if !ok {
					logSubscription(scope, topic).Str("reason", "pubsub_closed").Msg("subscription end")
					return
				}
				v, ok := data.(S)
				if !ok {
					continue
				}
				converted, keep := convert(v)
				if !keep {
					continue
				}
				// 送信もキャンセル可能にする。out はバッファ1なので、gqlgen 側が
				// 読まないまま2件目が来るとここで止まる。以前は素の `out <- converted`
				// だったため、その状態で購読が切れても ctx.Done() へ戻れず、
				// goroutine と PubSub の購読が永久に残っていた（切断済みクライアント
				// ぶんだけ溜まる）。
				select {
				case out <- converted:
				case <-ctx.Done():
					logSubscription(scope, topic).Str("reason", "context_done_blocked_send").Msg("subscription end")
					return
				}
			}
		}
	}()

	return out
}

// logSubscription は購読の開始・終了ログを同じ形で出す。
// フィールドはメッセージ購読が以前から出していたもの（room_id / user_id / topic）に
// 揃えてある。ルームを持たない購読では room_id を省く。
func logSubscription(scope subscriptionScope, topic string) *zerolog.Event {
	ev := logger.Log.Info()
	if scope.RoomID != "" {
		ev = ev.Str("room_id", scope.RoomID)
	}
	return ev.Int64("user_id", scope.UserID).Str("topic", topic)
}
