package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/redis/go-redis/v9"
)

// sseFanoutChannel は SSE の配信に使う Redis のチャンネル。
//
// 利用者ごとにチャンネルを分けていない。分けると「この台に繋がっている利用者」の
// ぶんだけ購読を張り替え続けることになり、接続・切断のたびに Redis への
// SUBSCRIBE/UNSUBSCRIBE が走る。SSE は1利用者あたり数本、台あたり数千本という
// 規模なので、1本のチャンネルに流して各台が自分の繋がっている相手ぶんだけ
// 拾う方が単純で速い。
const sseFanoutChannel = "sse:fanout"

// SSEFanout は SSE のイベントを全ての台へ配る sse.Fanout。
//
// SSE の接続はどこか1台に貼り付くが、イベントを起こす操作（通知の作成、
// メッセージの送信）は別の台に当たりうる。プロセス内の配信だけだと、
// 通知を作った台に繋がっていない利用者にはベルが光らない。しかも「届かない」
// だけでエラーにはならないので、1台で動かしている間は誰も気づけない。
//
//	Publish → Redis PUBLISH → (全台) Redis SUBSCRIBE → 各台の Broker → クライアント
//
// 自分の台のクライアントにも、Redis から戻ってきたものを渡す。直接渡す近道を
// 作ると、送った台の購読者にだけ二重に届く（これも1台では起きない）。
type SSEFanout struct {
	client *redis.Client
	prefix string

	ctx    context.Context
	cancel context.CancelFunc
	sub    *redis.PubSub
	wg     sync.WaitGroup
}

var _ sse.Fanout = (*SSEFanout)(nil)

// NewSSEFanout は配信口を作り、受信を始める。
// deliver は受け取ったイベントをこの台のクライアントへ渡す関数（Broker.DeliverLocal）。
func NewSSEFanout(client *redis.Client, prefix string, deliver func(sse.Envelope)) *SSEFanout {
	ctx, cancel := context.WithCancel(context.Background())
	f := &SSEFanout{client: client, prefix: prefix, ctx: ctx, cancel: cancel}
	f.sub = client.Subscribe(ctx, prefix+sseFanoutChannel)

	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		ch := f.sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var env sse.Envelope
				if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
					// 1件壊れていても受信は止めない。止めると、その後の全配信が
					// 静かに死ぬ。
					logger.Log.Error().Err(err).
						Str("component", "redis_sse_fanout").
						Msg("failed to decode a fanned-out SSE event; dropping it")
					continue
				}
				deliver(env)
			}
		}
	}()
	return f
}

func (f *SSEFanout) Publish(ctx context.Context, env sse.Envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to encode an SSE event: %w", err)
	}
	return f.client.Publish(ctx, f.prefix+sseFanoutChannel, raw).Err()
}

// Close は受信を止める。サーバーの停止時に呼ぶ。
func (f *SSEFanout) Close() error {
	f.cancel()
	err := f.sub.Close()
	f.wg.Wait()
	return err
}
