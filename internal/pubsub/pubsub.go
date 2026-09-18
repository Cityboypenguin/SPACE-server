package pubsub

import (
	"sync"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

type PubSub struct {
	mu   sync.RWMutex
	subs map[string][]chan interface{}
}

func New() *PubSub {
	return &PubSub{subs: make(map[string][]chan interface{})}
}

func (ps *PubSub) Subscribe(topic string) chan interface{} {
	ch := make(chan interface{}, 64)
	ps.mu.Lock()
	ps.subs[topic] = append(ps.subs[topic], ch)
	count := len(ps.subs[topic])
	ps.mu.Unlock()
	logger.Log.Debug().Str("topic", topic).Int("subscribers", count).Msg("pubsub subscribe")
	return ch
}

func (ps *PubSub) Unsubscribe(topic string, ch chan interface{}) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	subs := ps.subs[topic]
	for i, s := range subs {
		if s == ch {
			remaining := append(subs[:i], subs[i+1:]...)
			// 最後の購読者が抜けたらトピックごと消す。以前は削除した直後に return
			// していたため関数末尾の delete に到達せず、空スライスが map に残り続けた。
			// トピック名はルームID・メッセージID・質問IDごとに作られるので、
			// 見た誰かぶんのキーが永久に積み上がる（プロセスが生きている限り解放
			// されない）。同じロックの中で消すので、Publish 側から中途半端な状態は
			// 見えない。
			if len(remaining) == 0 {
				delete(ps.subs, topic)
			} else {
				ps.subs[topic] = remaining
			}
			logger.Log.Debug().Str("topic", topic).Int("subscribers", len(remaining)).Msg("pubsub unsubscribe")
			close(ch)
			return
		}
	}
}

func (ps *PubSub) Publish(topic string, data interface{}) {
	ps.mu.RLock()
	subs := make([]chan interface{}, len(ps.subs[topic]))
	copy(subs, ps.subs[topic])
	ps.mu.RUnlock()
	logger.Log.Debug().Str("topic", topic).Int("subscribers", len(subs)).Msg("pubsub publish")
	for _, ch := range subs {
		func(ch chan interface{}) {
			defer func() {
				_ = recover()
			}()
			select {
			case ch <- data:
			default:
				logger.Log.Warn().Str("topic", topic).Msg("pubsub dropped message for slow subscriber")
			}
		}(ch)
	}
}
