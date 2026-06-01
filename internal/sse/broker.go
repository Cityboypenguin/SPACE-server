package sse

import "sync"

type Event struct {
	ID   int            `json:"id"`
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
	Time string         `json:"time"`
}

type Client struct {
	ch chan Event
}

// Broker はユーザーごとの SSE クライアント接続を管理し、イベントを配信する。
type Broker struct {
	mu      sync.Mutex
	clients map[int64][]*Client
	nextID  int64
}

func NewBroker() *Broker {
	return &Broker{
		clients: make(map[int64][]*Client),
		nextID:  1,
	}
}

func (b *Broker) Subscribe(userID int64) *Client {
	c := &Client{ch: make(chan Event, 32)}
	b.mu.Lock()
	b.clients[userID] = append(b.clients[userID], c)
	b.mu.Unlock()
	return c
}

func (b *Broker) Unsubscribe(userID int64, c *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.clients[userID]
	for i, cl := range list {
		if cl == c {
			close(c.ch)
			b.clients[userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(b.clients[userID]) == 0 {
		delete(b.clients, userID)
	}
}

func (b *Broker) PublishToUser(userID int64, eventType string, data map[string]any) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	clients := make([]*Client, len(b.clients[userID]))
	copy(clients, b.clients[userID])
	b.mu.Unlock()

	ev := Event{ID: int(id), Type: eventType, Data: data}
	for _, c := range clients {
		select {
		case c.ch <- ev:
		default:
		}
	}
}
