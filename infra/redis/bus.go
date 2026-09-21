package redis

import (
	"context"
	"sync"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/redis/go-redis/v9"
)

// Bus は GraphQL の subscription を台またぎで配る pubsub.Bus。
//
// GraphQL の subscription は WebSocket なので、購読者は必ずどこか1台に貼り付く。
// 一方メッセージを送る利用者は別の台に当たりうる。プロセス内の PubSub だけだと、
// 送った台に繋がっている購読者にしか届かない。しかも「届かない」だけで
// エラーにはならないので、1台で動かしている間は誰も気づけない。
//
// 作りは単純で、配信は必ず Redis を経由させる。自分の台の購読者にも、
// Redis から戻ってきたものを渡す。
//
//	Publish → Redis PUBLISH → (全台) Redis SUBSCRIBE → プロセス内 PubSub → 購読者
//
// ローカルへ直接渡す近道を作らないのは、同じ値が2回届くのを防ぐため。近道と
// Redis 経由の両方を通すと、送った台の購読者にだけ二重に届く（そしてこれも
// 1台では起きない）。
type Bus struct {
	local  *pubsub.PubSub
	client *redis.Client
	codec  *pubsub.Codec
	prefix string

	ctx    context.Context
	cancel context.CancelFunc

	// sub は Redis 側の購読。トピックは購読者が現れてから増やし、
	// 最後の1人が抜けたら減らす（全トピックを常時購読すると、この台に
	// 誰も見ていない部屋のメッセージまで運ぶことになる）。
	sub *redis.PubSub

	// mu は refs と、それに対応する Redis 側の購読状態をまとめて守る。
	//
	// Redis への SUBSCRIBE / UNSUBSCRIBE も、この錠を持ったまま行う。以前は
	// 数を更新してから錠を放し、その外で Redis を呼んでいたため、購読と解除が
	// 重なると順番が入れ替わりえた。
	//
	//	A: refs 0→1（購読する、と決める）
	//	B: refs 1→0（解除する、と決める）
	//	B: Redis UNSUBSCRIBE
	//	A: Redis SUBSCRIBE
	//
	// この並びなら結果は正しいが、逆に走れば「購読者が居るのに Redis 側は
	// 解除済み」になる。そうなるとその購読には他の台からの配信が二度と届かず、
	// しかもエラーにならないので気づけない。数と Redis の状態は1つの不変条件
	// なので、同じ錠の下で動かす必要がある。
	//
	// 錠の中で Redis を1往復することになるが、ここは購読の開始・終了でしか
	// 通らない（配信 Publish も受信 receive もこの錠を取らない）ので、
	// 1メッセージごとの費用にはならない。
	mu sync.Mutex
	// refs はトピックごとのプロセス内購読者数。Redis 側の購読・解除を
	// 0↔1 の境目でだけ行うために数える。
	refs map[string]int

	wg sync.WaitGroup
}

var _ pubsub.Bus = (*Bus)(nil)

// NewBus は Redis 経由の Bus を作り、受信を始める。
//
// prefix は Redis のチャンネル名の頭に付ける文字列。同じ Redis を別の環境
// （検証と本番）で共有してしまったときに混ざらないようにするためのもの。
func NewBus(client *redis.Client, codec *pubsub.Codec, prefix string) *Bus {
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bus{
		local:  pubsub.New(),
		client: client,
		codec:  codec,
		prefix: prefix,
		ctx:    ctx,
		cancel: cancel,
		refs:   make(map[string]int),
	}
	// チャンネルを1つも指定せずに始める。go-redis はこの状態を許し、
	// あとから Subscribe で足せる。
	b.sub = client.Subscribe(ctx)

	b.wg.Add(1)
	go b.receive()
	return b
}

func (b *Bus) channel(topic string) string { return b.prefix + topic }

// receive は Redis から届いた値をプロセス内の PubSub へ流し込む。
func (b *Bus) receive() {
	defer b.wg.Done()
	ch := b.sub.Channel()
	for {
		select {
		case <-b.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			data, err := b.codec.Decode([]byte(msg.Payload))
			if err != nil {
				// 知らない型・壊れた値で受信を止めない。止めると、その後の
				// 全トピックの配信が静かに死ぬ。
				logger.Log.Error().Err(err).
					Str("component", "redis_bus").
					Str("channel", msg.Channel).
					Msg("failed to decode a published value; dropping it")
				continue
			}
			b.local.Publish(b.topicOf(msg.Channel), data)
		}
	}
}

func (b *Bus) topicOf(channel string) string {
	if len(channel) >= len(b.prefix) && channel[:len(b.prefix)] == b.prefix {
		return channel[len(b.prefix):]
	}
	return channel
}

func (b *Bus) Subscribe(topic string) chan interface{} {
	// プロセス内の購読を先に作る。Redis 側の購読が有効になるまでの間に
	// 届いた値を取りこぼさないため（逆順にすると、その隙間で来たものが
	// 「購読者ゼロ」として捨てられる）。
	ch := b.local.Subscribe(topic)

	b.mu.Lock()
	defer b.mu.Unlock()

	first := b.refs[topic] == 0
	b.refs[topic]++
	if !first {
		return ch
	}

	if err := b.sub.Subscribe(b.ctx, b.channel(topic)); err != nil {
		// 失敗しても数は戻さず、解除の責任も手放さない。go-redis は
		// SUBSCRIBE が失敗しても「このチャンネルを購読しているつもり」の一覧に
		// 加え、接続を張り直すたびにその一覧をまとめて購読し直す。つまり
		// 回復はライブラリ側が勝手にやるので、こちらで数え直す必要はない。
		//
		// 逆に「失敗したから購読していない」と覚えて Unsubscribe を省くと、
		// 一覧には残ったままになり、再接続のたびに購読が蘇る。誰も読んでいない
		// トピックの配信をこの台が永久に受け取り続けることになる。
		//
		// 配信が戻るまでの間、この購読には**何も届かない**。以前ここには
		// 「同じ台からの配信は届く」と書いてあったが、それは誤り。Publish は
		// 必ず Redis を経由する（同じ値が二重に届くのを避けるため、
		// ローカルへ直接渡す近道は意図的に作っていない）ので、Redis 側の購読が
		// 無ければ自分の台で起きたイベントも戻ってこない。
		logger.Log.Error().Err(err).
			Str("component", "redis_bus").
			Str("topic", topic).
			Msg("failed to subscribe on redis; this subscription receives nothing until go-redis re-establishes the connection")
	}
	return ch
}

func (b *Bus) Unsubscribe(topic string, ch chan interface{}) {
	b.local.Unsubscribe(topic, ch)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.refs[topic] > 0 {
		b.refs[topic]--
	}
	if b.refs[topic] > 0 {
		return
	}

	// 最後の1人が抜けた。記録を先に消すのは、この後の UNSUBSCRIBE が失敗しても
	// 「購読者ゼロ」という事実は変わらないため。
	delete(b.refs, topic)
	if err := b.sub.Unsubscribe(b.ctx, b.channel(topic)); err != nil {
		logger.Log.Warn().Err(err).
			Str("component", "redis_bus").
			Str("topic", topic).
			Msg("failed to unsubscribe on redis")
	}
}

func (b *Bus) Publish(topic string, data interface{}) {
	payload, err := b.codec.Encode(data)
	if err != nil {
		logger.Log.Error().Err(err).
			Str("component", "redis_bus").
			Str("topic", topic).
			Msg("failed to encode a value; not publishing")
		return
	}
	if err := b.client.Publish(b.ctx, b.channel(topic), payload).Err(); err != nil {
		// 配信は取りこぼしても画面が壊れない（クライアントは再取得できる）ので、
		// 呼び出し側を失敗させない。プロセス内 PubSub の Publish も同じく
		// エラーを返さないので、差し替えても呼び出し側は変わらない。
		logger.Log.Error().Err(err).
			Str("component", "redis_bus").
			Str("topic", topic).
			Msg("failed to publish")
	}
}

// Close は受信を止める。サーバーの停止時に呼ぶ。
func (b *Bus) Close() error {
	b.cancel()
	err := b.sub.Close()
	b.wg.Wait()
	return err
}
