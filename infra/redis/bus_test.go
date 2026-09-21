package redis

import (
	"sync"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// busPayload は配信を確かめるためだけの値。
type busPayload struct {
	ID string `json:"id"`
}

func newTestCodec() *pubsub.Codec {
	c := pubsub.NewCodec()
	c.Register("bus_test_payload", (*busPayload)(nil))
	return c
}

func newTestBus(t *testing.T, srv *miniredis.Miniredis) *Bus {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	bus := NewBus(client, newTestCodec(), "test:")
	t.Cleanup(func() {
		_ = bus.Close()
		_ = client.Close()
	})
	return bus
}

// mustReceive は「届くはず」を確かめる。
//
// Redis への SUBSCRIBE は投げっぱなしなので、購読が有効になるまでに少し間がある。
// 一度だけ配信して待つと、その間に当たったときだけ落ちる不安定なテストになるので、
// 届くまで配り直す。購読が本当に外れていれば何度配っても届かないので、
// 確かめたい性質（購読が生きているか）は緩まない。
func mustReceive(t *testing.T, bus *Bus, topic string, ch chan interface{}, id string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		bus.Publish(topic, &busPayload{ID: id})
		select {
		case got := <-ch:
			p, ok := got.(*busPayload)
			if !ok || p.ID != id {
				t.Fatalf("受け取った値が違う: %+v", got)
			}
			return
		case <-time.After(20 * time.Millisecond):
		case <-deadline:
			t.Fatalf("%q が届かない（Redis 側の購読が外れている）", topic)
		}
	}
}

// 別の台で配ったものが届くこと（Bus を置いた理由そのもの）。
func TestBus_DeliversAcrossInstances(t *testing.T) {
	srv := miniredis.RunT(t)
	publisher := newTestBus(t, srv)
	subscriber := newTestBus(t, srv)

	ch := subscriber.Subscribe("room-1:message:added")
	defer subscriber.Unsubscribe("room-1:message:added", ch)

	mustReceive(t, publisher, "room-1:message:added", ch, "m1")
}

// 同じトピックの購読者が2人居るとき、1人が抜けてももう1人へ届き続けること。
//
// Redis 側の購読は「0人→1人」で張り、「1人→0人」で外す。数の数え方を誤ると、
// 1人抜けただけで Redis 側が外れ、残った購読者に他の台からの配信が
// 届かなくなる。しかもエラーは出ないので、気づく手がかりが無い。
func TestBus_KeepsTheRedisSubscriptionWhileOthersRemain(t *testing.T) {
	srv := miniredis.RunT(t)
	publisher := newTestBus(t, srv)
	subscriber := newTestBus(t, srv)

	const topic = "room-1:message:added"
	first := subscriber.Subscribe(topic)
	second := subscriber.Subscribe(topic)

	subscriber.Unsubscribe(topic, first)

	mustReceive(t, publisher, topic, second, "m1")
	subscriber.Unsubscribe(topic, second)
}

// 購読と解除が重なっても、最後に残った購読者へ届くこと。
//
// 以前は数の更新と Redis の購読・解除が別の錠の外で行われていたので、
//
//	A: 数 0→1（張ると決める）
//	B: 数 1→0（外すと決める）
//	B: Redis UNSUBSCRIBE
//	A: Redis SUBSCRIBE
//
// のように順番が入れ替わりえた。逆に走れば「購読者が居るのに Redis 側は解除済み」
// という状態で固定される。-race では見つからない種類の壊れ方なので、
// 最後に実際に配って確かめる。
func TestBus_SurvivesConcurrentSubscribeAndUnsubscribe(t *testing.T) {
	srv := miniredis.RunT(t)
	publisher := newTestBus(t, srv)
	subscriber := newTestBus(t, srv)

	const topic = "room-1:message:added"

	// 最後まで残る購読者。この1本が生きている限り、Redis 側の購読も
	// 外れていてはいけない。
	survivor := subscriber.Subscribe(topic)
	defer subscriber.Unsubscribe(topic, survivor)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := subscriber.Subscribe(topic)
			subscriber.Unsubscribe(topic, ch)
		}()
	}
	wg.Wait()

	mustReceive(t, publisher, topic, survivor, "m1")
}

// 全員が抜けたあと、また購読すれば届くこと（Redis 側を張り直せていること）。
func TestBus_ResubscribesAfterEveryoneLeft(t *testing.T) {
	srv := miniredis.RunT(t)
	publisher := newTestBus(t, srv)
	subscriber := newTestBus(t, srv)

	const topic = "room-1:message:added"
	first := subscriber.Subscribe(topic)
	subscriber.Unsubscribe(topic, first)

	second := subscriber.Subscribe(topic)
	defer subscriber.Unsubscribe(topic, second)

	mustReceive(t, publisher, topic, second, "m1")
}

// 知らない型が流れてきても受信が止まらないこと。
//
// 止まると、その台の全トピックの配信が静かに死ぬ。1件の壊れた値で
// チャット全体が無言になるのは、壊れ方として重すぎる。
func TestBus_KeepsReceivingAfterAnUndecodableValue(t *testing.T) {
	srv := miniredis.RunT(t)
	publisher := newTestBus(t, srv)
	subscriber := newTestBus(t, srv)

	const topic = "room-1:message:added"
	ch := subscriber.Subscribe(topic)
	defer subscriber.Unsubscribe(topic, ch)

	// Codec を通さずに直接流す＝受け手は復元できない。
	publisher.client.Publish(publisher.ctx, publisher.channel(topic), "not an envelope")

	mustReceive(t, publisher, topic, ch, "m1")
}
