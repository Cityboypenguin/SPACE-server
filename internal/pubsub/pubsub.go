package pubsub

import (
	"log"
	"sync"
)

type PubSub struct {
	mu   sync.RWMutex
	subs map[string][]chan interface{}
}

func New() *PubSub {
	return &PubSub{subs: make(map[string][]chan interface{})}
}

func (ps *PubSub) Subscribe(topic string) chan interface{} {
	ch := make(chan interface{}, 64) // バッファサイズは適宜調整
	ps.mu.Lock()
	ps.subs[topic] = append(ps.subs[topic], ch)
	count := len(ps.subs[topic])
	ps.mu.Unlock()
	log.Printf("[PubSub] subscribe topic=%s subscribers=%d", topic, count)
	return ch
}

func (ps *PubSub) Unsubscribe(topic string, ch chan interface{}) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	subs := ps.subs[topic]
	for i, s := range subs {
		if s == ch {
			ps.subs[topic] = append(subs[:i], subs[i+1:]...)
			log.Printf("[PubSub] unsubscribe topic=%s subscribers=%d", topic, len(ps.subs[topic]))
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
	log.Printf("[PubSub] publish topic=%s subscribers=%d", topic, len(subs))
	for _, ch := range subs {
		func(ch chan interface{}) {
			defer func() {
				_ = recover()
			}()
			select {
			case ch <- data:
			default:
				log.Printf("[PubSub] publish topic=%s dropped message for slow subscriber", topic)
			}
		}(ch)
	}
}
