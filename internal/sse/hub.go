package sse

import "sync"

// Event は SSE で配信するイベント
type Event struct {
	ID   int            `json:"id"`
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
	Time string         `json:"time"`
}

// Client は各接続の送信キュー
type Client struct {
	ch chan Event
}

// Hub はクライアント管理とブロードキャストを担当
type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
	nextID  int64
}

// NewHub は Hub を作成する
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		nextID:  1,
	}
}

// Subscribe は新しいクライアントを登録して返す
func (h *Hub) Subscribe() *Client {
	c := &Client{ch: make(chan Event, 32)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Unsubscribe はクライアントを削除する
func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.ch)
	}
	h.mu.Unlock()
}

// Publish は全クライアントへイベントを配信する（遅いクライアントは drop）
func (h *Hub) Publish(eventType string, data map[string]any) Event {
	h.mu.Lock()
	id := h.nextID
	h.nextID++

	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	ev := Event{
		ID:   int(id),
		Type: eventType,
		Data: data,
		// Time は handler 側で設定する
	}

	for _, c := range clients {
		select {
		case c.ch <- ev:
		default:
			// drop（要件次第で Unsubscribe しても良い）
		}
	}

	return ev
}
