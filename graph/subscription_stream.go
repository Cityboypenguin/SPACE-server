package graph

import (
	"context"
	"time"

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

// identity は「変換も絞り込みもしない」を表す convert。
// ほとんどの購読は PubSub に流れてきた値をそのまま配るだけなので、
// 呼び出し側に同じクロージャを書かせないために用意してある。
func identity[T any](v T) (T, bool) { return v, true }

// accessRecheckInterval は、購読中に権限を確かめ直す間隔。
//
// 権限は購読を開始するときに一度見ているが、購読は何時間も生き続ける。その間に
// 退出・キック・ブロックが起きても、ループ側が何も確かめなければ新着が流れ続ける。
// 「もう読めない部屋の新着だけが、繋ぎっぱなしのタブに届く」という形になる。
//
// 確かめ直すのは配信の直前だけ。部屋が静かなら1度も確かめないが、それで構わない
// （配信しないので漏れようがない）。逆に言えば、権限を失ってから実際に漏れるのは
// 「次の配信」までで、この間隔ぶんが上限になる。
//
// 毎回確かめないのは、賑やかな部屋では配信のたびに購読者の数だけ判定が走るため。
// 判定自体は EXISTS 1本（usecase/chat の IsRoomMember）だが、それを
// メッセージ数 × 購読者数だけ掛けると無視できない。
//
// const ではなく var なのはテストから短くするため（本番で書き換えてはいけない）。
// 30秒の待ちを挟むテストは、確かめたい性質のわりに遅すぎる。
var accessRecheckInterval = 30 * time.Second

// subscribeTopicFunc はルームに紐づかない購読のための入口。権限を確かめ直さない。
//
// 使ってよいのは「購読を始めてよいか」が接続中ずっと変わらない購読だけ
// （取り込み状況の購読のように、JWT の役割だけで決まるもの）。ルームの購読には
// 使わないこと。退出・キックで条件が変わるので、確かめ直さないと
// 読めなくなった部屋の新着が流れ続ける。そちらは subscribeTopicGuarded を使う。
//
// convert が false を返した値は配信しない。変換をループの中に持たせているのは、
// 変換の失敗や絞り込みで「1件送らない」時にループを止めてはいけないため
// （呼び出し側で map しようとすると、そのたびにもう1本ループを書くことになる）。
func subscribeTopicFunc[S any, T any](
	ctx context.Context,
	src subscriptionSource,
	topic string,
	scope subscriptionScope,
	convert func(S) (T, bool),
) <-chan T {
	return subscribeTopicGuarded(ctx, src, topic, scope, nil, convert)
}

// subscribeTopicGuarded は subscribeTopicFunc に「配信し続けてよいかの確認」を足した版。
//
// authorize は購読開始時と同じ権限判定を渡す（nil なら確認しない。ルームに
// 紐づかない購読――取り込み状況のように、開始時の認証だけで決まるもの――で使う）。
// エラーを返した時点で購読を終わらせる。チャンネルを閉じると gqlgen 側が
// subscription を完了させるので、クライアントからは配信が止まって見える。
//
// 確認は「配信する値が来たとき」かつ「前回の確認から accessRecheckInterval 以上
// 経っているとき」だけ行う。絞り込みで落とす値では確認しない（配信しないものの
// ために判定を走らせる理由が無い）。
func subscribeTopicGuarded[S any, T any](
	ctx context.Context,
	src subscriptionSource,
	topic string,
	scope subscriptionScope,
	authorize func(context.Context) error,
	convert func(S) (T, bool),
) <-chan T {
	logSubscription(scope, topic).Msg("subscription start")

	// バッファ1は従来どおり。gqlgen 側の読み出しが遅れても配信元
	// （pubsub.Publish）を止めないための1段で、詰まればここでブロックする。
	out := make(chan T, 1)
	sub := src.Subscribe(topic)

	// 開始時に権限を確かめたばかりなので、そこを起点に数える。
	lastChecked := time.Now()

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
				// 配信する直前に、まだ読んでよいかを確かめ直す。
				if authorize != nil && time.Since(lastChecked) >= accessRecheckInterval {
					if err := authorize(ctx); err != nil {
						logSubscription(scope, topic).
							Err(err).
							Str("reason", "access_revoked").
							Msg("subscription end")
						return
					}
					lastChecked = time.Now()
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
