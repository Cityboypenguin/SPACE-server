package chat

import (
	"github.com/Cityboypenguin/SPACE-server/internal/async"
)

// AsyncRunner はチャット配信の一部をリクエストの応答時間の外へ出すための実行口。
//
// 実体は internal/async.Runner（投げっぱなしの流儀はサーバ全体で1つ。ctx の扱い・
// panic の握り・停止時の待ち合わせはあちらのコメント参照）。ここに残しているのは
// 「チャットの配信のうち何をそこへ渡してよいか」という、チャット側の判断。
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
//     取り直すだけ）ので、入れ替わっても壊れない。ここを Runner へ渡す。
//
// # 以前の実装との違い
//
// 以前はルーム単位のFIFOキューとワーカー、積み残し上限を持つ decorator だった。
// 順序を保つ機構が要ったのは、送信のたびに走る未読集計
// （履修者×メッセージの JOIN）を非同期へ逃がすために配信を丸ごと外へ出しており、
// その中に順序の要る PubSub も混ざっていたから。集計そのものをやめ、順序の要る
// PubSub を同期に戻したので、キューで直列化する理由が無くなった。
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
type AsyncRunner = async.Runner

// NewAsyncRunner はチャット配信用の Runner を返す。
//
// component 名は配信ログ（graph/chat_events.go の logChatDelivery）と揃えてある。
// 非同期の中で panic したときも、同じ component で追えるようにするため。
func NewAsyncRunner() *AsyncRunner { return async.NewRunner("chat_event_publisher") }
