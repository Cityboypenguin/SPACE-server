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

// Hub はユーザーごとのクライアント管理と配信を担当
type Hub struct {
	mu      sync.Mutex
	clients map[int64][]*Client // userID → 接続中クライアント一覧
	nextID  int64
}

// NewHub は Hub を作成する
func NewHub() *Hub {
	return &Hub{
		clients: make(map[int64][]*Client),
		nextID:  1,
	}
}

// Subscribe は指定ユーザーの新しいクライアントを登録して返す
func (h *Hub) Subscribe(userID int64) *Client {
	c := &Client{ch: make(chan Event, 32)}
	h.mu.Lock()
	h.clients[userID] = append(h.clients[userID], c)
	h.mu.Unlock()
	return c
}

// Unsubscribe はクライアントを削除してチャネルを閉じる
func (h *Hub) Unsubscribe(userID int64, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.clients[userID]
	for i, cl := range list {
		if cl == c {
			close(c.ch)
			h.clients[userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(h.clients[userID]) == 0 {
		delete(h.clients, userID)
	}
}

// PublishToUser は特定ユーザーの全クライアントへイベントを配信する（遅いクライアントは drop）
func (h *Hub) PublishToUser(userID int64, eventType string, data map[string]any) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	clients := make([]*Client, len(h.clients[userID]))
	copy(clients, h.clients[userID])
	h.mu.Unlock()

	ev := Event{ID: int(id), Type: eventType, Data: data}
	for _, c := range clients {
		select {
		case c.ch <- ev:
		default:
		}
	}
}
