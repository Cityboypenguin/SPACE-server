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
			ps.subs[topic] = append(subs[:i], subs[i+1:]...)
			logger.Log.Debug().Str("topic", topic).Int("subscribers", len(ps.subs[topic])).Msg("pubsub unsubscribe")
			close(ch)
			return
		}
	}
	if len(ps.subs[topic]) == 0 {
		delete(ps.subs, topic)
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
