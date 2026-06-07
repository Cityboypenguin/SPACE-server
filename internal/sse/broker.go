package sse

import (
	"sync"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

const historySize = 100

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
	history map[int64][]Event // ユーザーごとの直近イベント履歴（再接続時リプレイ用）
	nextID  int64
}

func NewBroker() *Broker {
	return &Broker{
		clients: make(map[int64][]*Client),
		history: make(map[int64][]Event),
		nextID:  1,
	}
}

// Subscribe はクライアントを登録し、lastEventID より新しい未配信イベントを返す。
// lastEventID < 0 は初回接続（リプレイなし）を意味する。
//
// 注意: lastEventID が履歴の最古 ID より小さい場合（100件超の切断）は、
// 履歴に残っている範囲のみリプレイされる。それ以前のイベントは DB の通知一覧で確認できる。
func (b *Broker) Subscribe(userID int64, lastEventID int) (*Client, []Event) {
	c := &Client{ch: make(chan Event, 32)}
	b.mu.Lock()
	var missed []Event
	if lastEventID >= 0 {
		// サーバー再起動後は nextID が 1 から始まるため、クライアントの lastEventID が
		// 現在の nextID 以上であれば再起動を検出できる。この場合は履歴が空のため
		// リプレイは起きないが、sync イベントで未読数は補正される。
		if int64(lastEventID) >= b.nextID {
			logger.Log.Warn().
				Int64("userID", userID).
				Int("lastEventID", lastEventID).
				Int64("currentNextID", b.nextID).
				Msg("SSE reconnect after server restart: Last-Event-ID exceeds server counter, skipping replay")
		} else {
			hist := b.history[userID]
			// lastEventID が履歴の保持範囲外（100件超の切断）かチェックしてログ警告
			if len(hist) > 0 && hist[0].ID > lastEventID+1 {
				logger.Log.Warn().
					Int64("userID", userID).
					Int("lastEventID", lastEventID).
					Int("oldestHistoryID", hist[0].ID).
					Msg("SSE replay gap: some events evicted from history; client may have missed notifications")
			}
			for _, ev := range hist {
				if ev.ID > lastEventID {
					missed = append(missed, ev)
				}
			}
		}
	}
	b.clients[userID] = append(b.clients[userID], c)
	b.mu.Unlock()
	return c, missed
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

// PublishSyncToUser は現在の未読数をオンライン中のクライアントにのみ送信する。
// sync イベントは常に最新の DB 値を使うため、履歴には追加しない。
func (b *Broker) PublishSyncToUser(userID int64, unreadCount int) {
	ev := Event{
		Type: "sync",
		Data: map[string]any{"unreadCount": unreadCount},
	}
	b.mu.Lock()
	clients := make([]*Client, len(b.clients[userID]))
	copy(clients, b.clients[userID])
	b.mu.Unlock()

	for _, c := range clients {
		select {
		case c.ch <- ev:
		default:
		}
	}
}

// Broadcast は現在接続中の全ユーザーにイベントを送信する。
// terms_updated など全員対象のシステムイベントに使う。履歴には記録しない。
func (b *Broker) Broadcast(eventType string, data map[string]any) {
	ev := Event{Type: eventType, Data: data}
	b.mu.Lock()
	var all []*Client
	for _, clients := range b.clients {
		all = append(all, clients...)
	}
	b.mu.Unlock()

	for _, c := range all {
		select {
		case c.ch <- ev:
		default:
		}
	}
}

func (b *Broker) PublishToUser(userID int64, eventType string, data map[string]any) {
	b.mu.Lock()
	id := b.nextID
	b.nextID++

	ev := Event{ID: int(id), Type: eventType, Data: data}

	// 履歴に追加（直近 historySize 件を保持）
	hist := append(b.history[userID], ev)
	if len(hist) > historySize {
		hist = hist[len(hist)-historySize:]
	}
	b.history[userID] = hist

	clients := make([]*Client, len(b.clients[userID]))
	copy(clients, b.clients[userID])
	b.mu.Unlock()

	for _, c := range clients {
		select {
		case c.ch <- ev:
		default:
			// チャンネルバッファ満杯。イベントは履歴に残るため次回再接続時にリプレイされる。
			logger.Log.Warn().
				Int64("userID", userID).
				Int("eventID", int(id)).
				Str("eventType", eventType).
				Msg("SSE channel buffer full: event dropped for active client, will replay on reconnect")
		}
	}
}
