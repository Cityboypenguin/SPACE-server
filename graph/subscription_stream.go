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
// 退出・キックが起きても、ループ側が何も確かめなければ新着が流れ続ける。
// 「もう読めない部屋の新着だけが、繋ぎっぱなしのタブに届く」という形になる。
//
// これは**取りこぼしの受け皿**であって、権限が変わったときの第一の手段ではない。
// 第一の手段は subscriptionGuard.revokeTopic（権限が変わった側から合図を出し、
// 受け取った購読がその場で確かめ直す）で、そちらは秒を待たずに効く。
// ここが要るのは、合図を出し忘れた経路・合図が届かなかったとき（Redis の
// 瞬断）・DB を直接いじったときのため。つまり「合図が来なくても、いずれは
// 気づく」ための上限を決めているだけ。
//
// 確かめ直すのは配信の直前だけ。部屋が静かなら1度も確かめないが、それで構わない
// （配信しないので漏れようがない）。
//
// 毎回確かめないのは、賑やかな部屋では配信のたびに購読者の数だけ判定が走るため。
// 判定自体は EXISTS 1本（usecase/chat の IsRoomMember）だが、それを
// メッセージ数 × 購読者数だけ掛けると無視できない。
//
// const ではなく var なのはテストから短くするため（本番で書き換えてはいけない）。
// 30秒の待ちを挟むテストは、確かめたい性質のわりに遅すぎる。
var accessRecheckInterval = 30 * time.Second

// subscriptionGuard は「この購読を続けてよいか」の判定と、その判定を急がせる合図。
//
// authorize だけでは、権限を失ってから気づくまでに accessRecheckInterval かかる。
// revokeTopic は、権限を変えた側（退出・キックの処理）が「このルームのこの人たちを
// 確かめ直せ」と伝えるためのトピック。合図を受けた購読は間隔を待たずに authorize を
// 引き直し、通らなければその場で終わる。
//
// 合図そのものを「切れ」という命令にはしていない。合図が言えるのは「変わった」まで
// で、結果どうなったか（キックされたのか、役割が変わっただけか、すぐ入り直したか）
// を決めるのは判定1本（requireRoomReadAccess）でなければならない。合図に判断を
// 持たせると、入口の条件と居続けてよい条件がまた2箇所に分かれる。
type subscriptionGuard struct {
	// authorize は購読開始時と同じ権限判定。nil なら確かめ直さない
	// （ルームに紐づかない購読――取り込み状況のように、開始時の認証だけで
	// 決まるもの――で使う）。
	authorize func(context.Context) error
	// revokeTopic は権限が変わったことを知らせるトピック。空なら合図を待たない。
	revokeTopic string
}

// RoomAccessChanged は「このルームの、この利用者たちの閲覧権限が変わったかもしれない」
// という合図。中身は対象の利用者IDだけで、どう変わったかは載せない
// （判断は受け取った側が authorize でやり直す。subscriptionGuard のコメント参照）。
//
// 台をまたぐので pubsub の Codec に登録してある（graph/pubsub_types.go）。
type RoomAccessChanged struct {
	UserIDs []int64 `json:"userIDs"`
}

// affects はこの合図が userID に当たるか。UserIDs が空なら「このルーム全員」。
func (c *RoomAccessChanged) affects(userID int64) bool {
	if len(c.UserIDs) == 0 {
		return true
	}
	for _, id := range c.UserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

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
	return subscribeTopicGuarded(ctx, src, topic, scope, subscriptionGuard{}, convert)
}

// subscribeTopicGuarded は subscribeTopicFunc に「配信し続けてよいかの確認」を足した版。
//
// guard.authorize は購読開始時と同じ権限判定を渡す（ゼロ値なら確認しない。ルームに
// 紐づかない購読――取り込み状況のように、開始時の認証だけで決まるもの――で使う）。
// エラーを返した時点で購読を終わらせる。チャンネルを閉じると gqlgen 側が
// subscription を完了させるので、クライアントからは配信が止まって見える。
//
// 確認が走るのは次の2つ。
//
//   - guard.revokeTopic に自分宛の合図が来たとき（即時。退出・キックはこちら）
//   - 配信する値が来たとき、かつ前回の確認から accessRecheckInterval 以上
//     経っているとき（取りこぼしの受け皿）
//
// 絞り込みで落とす値では確認しない（配信しないもののために判定を走らせる理由が無い）。
func subscribeTopicGuarded[S any, T any](
	ctx context.Context,
	src subscriptionSource,
	topic string,
	scope subscriptionScope,
	guard subscriptionGuard,
	convert func(S) (T, bool),
) <-chan T {
	logSubscription(scope, topic).Msg("subscription start")

	// バッファ1は従来どおり。gqlgen 側の読み出しが遅れても配信元
	// （pubsub.Publish）を止めないための1段で、詰まればここでブロックする。
	out := make(chan T, 1)
	sub := src.Subscribe(topic)

	// 合図の購読。判定が無い購読では張らない（nil のチャンネルは select で
	// 永久に待つだけなので、下のループはそのまま書ける）。
	var revoked chan interface{}
	if guard.authorize != nil && guard.revokeTopic != "" {
		revoked = src.Subscribe(guard.revokeTopic)
	}

	// 開始時に権限を確かめたばかりなので、そこを起点に数える。
	lastChecked := time.Now()

	// recheck は権限を引き直し、通らなければ購読を終わらせてよいかを返す。
	recheck := func() bool {
		if err := guard.authorize(ctx); err != nil {
			logSubscription(scope, topic).
				Err(err).
				Str("reason", "access_revoked").
				Msg("subscription end")
			return false
		}
		lastChecked = time.Now()
		return true
	}

	// handleSignal は合図1件を処理する。自分宛でなければ何もしない。
	// 戻り値は「購読を続けてよいか」。
	handleSignal := func(sig interface{}) bool {
		// 型が違うものが流れてきたら、自分宛かどうか判断できない。
		// 判断できない＝確かめる側へ倒す（黙って配り続けない）。
		if c, ok := sig.(*RoomAccessChanged); ok && !c.affects(scope.UserID) {
			return true
		}
		return recheck()
	}

	// drainSignals は「もう届いている合図」を配信の直前に全て処理する。
	//
	// これが要るのは、select が複数の case を同時に選べるとき**無作為に**
	// 1つを選ぶため。キックの合図と新着メッセージが両方待っている状態では、
	// 合図が先に処理される保証がない。メッセージ側が選ばれ、かつ前回の確認から
	// accessRecheckInterval 以内なら、権限を失った後の1件が配信されてしまう
	// （合図で窓は縮むが、ゼロにはならない）。
	//
	// 配信する値を掴んでから送る手前でここを通せば、「その時点で届いていた合図」は
	// 必ず先に効く。届く前の合図まで効かせることはできないが、それは
	// 「メッセージが先に届いた」だけで、競合としては本物。
	drainSignals := func() bool {
		for {
			select {
			case sig, ok := <-revoked:
				if !ok {
					revoked = nil
					return true
				}
				if !handleSignal(sig) {
					return false
				}
			default:
				return true
			}
		}
	}

	go func() {
		// 後始末は defer に寄せる。終了地点が「受信待ちでの ctx 終了」「PubSub の
		// クローズ」「送信待ちでの ctx 終了」「権限の喪失」に増えたので、各 return の
		// 手前で close(out) を書き並べると閉じ忘れが必ず出る。LIFO なので
		// close(out) → Unsubscribe の順で走る。
		if revoked != nil {
			defer src.Unsubscribe(guard.revokeTopic, revoked)
		}
		defer src.Unsubscribe(topic, sub)
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				logSubscription(scope, topic).Str("reason", "context_done").Msg("subscription end")
				return
			case sig, ok := <-revoked:
				if !ok {
					// 合図のトピックだけが閉じた。本体の購読は生きているので
					// 続ける（以後は accessRecheckInterval の受け皿だけが効く）。
					revoked = nil
					continue
				}
				if !handleSignal(sig) {
					return
				}
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
				// 配信する直前に、届いている合図を先に片付ける。
				if revoked != nil && !drainSignals() {
					return
				}
				// そのうえで、間隔ぶん経っていればもう一度確かめ直す
				// （合図が来ない経路・届かなかったときの受け皿）。
				if guard.authorize != nil && time.Since(lastChecked) >= accessRecheckInterval {
					if !recheck() {
						return
					}
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
