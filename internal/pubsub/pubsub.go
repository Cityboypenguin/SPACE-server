package pubsub

import "sync"

type PubSub struct {
	mu   sync.RWMutex
	subs map[string][]chan interface{}
}

func New() *PubSub {
	return &PubSub{subs: make(map[string][]chan interface{})}
}

func (ps *PubSub) Subscribe(topic string) chan interface{} {
	ch := make(chan interface{}, 1)
	ps.mu.Lock()
	ps.subs[topic] = append(ps.subs[topic], ch)
	ps.mu.Unlock()
	return ch
}

func (ps *PubSub) Unsubscribe(topic string, ch chan interface{}) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	subs := ps.subs[topic]
	for i, s := range subs {
		if s == ch {
			ps.subs[topic] = append(subs[:i], subs[i+1:]...)
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
	for _, ch := range subs {
		select {
		case ch <- data:
		default:
		}
	}
}
