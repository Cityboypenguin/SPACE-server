package sse

import (
	"fmt"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/metrics"
)

const historySize = 100

// historyTTL は接続が無いユーザーのイベント履歴を保持する時間。
//
// 履歴の用途は「切断中に起きたぶんを再接続時にリプレイする」ことだけ。クライアントの
// 再接続はバックオフでも最大30秒、タブを隠したままでも60秒で畳んで復帰時に即つなぐ
// （SPACE-client の NotificationContext）ので、10分あれば普通の切断は全て拾える。
// これを超える切断（スリープ・長時間の離席）では、リプレイの代わりにクライアントが
// 通知一覧とルーム一覧を取り直す。通知の本体は DB にあるので取りこぼしにはならない
// （リプレイされた通知はトーストを出さない＝画面上の意味はほぼ無い）。
//
// 上限を設けるのは、以前はここが無制限だったため。接続していないユーザーぶんの履歴も
// 消さずに持ち続けており、一度でも通知を受けたユーザーの数だけメモリが増え続けていた。
const historyTTL = 10 * time.Minute

// historySweepInterval は履歴の掃除を走らせる最短間隔。
//
// 掃除は配信・切断のついでに行い、専用の goroutine は持たない（Broker に停止処理を
// 増やさないため。止め忘れた goroutine のほうが漏れとしては厄介）。掃除は履歴を
// 全てなめるが、走るのはこの間隔に1回だけ。
const historySweepInterval = time.Minute

// maxSSEConnectionsPerUser は1ユーザーが同時に保持できるSSE接続数の上限。
// 複数タブを考慮して5としている。IPではなくユーザーID単位のため大学NATでも問題ない。
const maxSSEConnectionsPerUser = 5

type Event struct {
	ID   int            `json:"id"`
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
	Time string         `json:"time"`
}

type Client struct {
	ch chan Event
}

// userHistory は1ユーザーぶんのイベント履歴と、最後に書き足した時刻。
// 時刻を持つのは、接続が無くなったユーザーの履歴を捨ててよいか判断するため。
type userHistory struct {
	events    []Event
	updatedAt time.Time
}

// Broker はユーザーごとの SSE クライアント接続を管理し、イベントを配信する。
type Broker struct {
	mu      sync.Mutex
	clients map[int64][]*Client
	history map[int64]*userHistory // ユーザーごとの直近イベント履歴（再接続時リプレイ用）
	nextID  int64

	// now はテストで時間を進めるための差し替え口（履歴の期限切れを待たずに試すため）。
	now func() time.Time
	// lastSweep は履歴の掃除を最後に走らせた時刻。
	lastSweep time.Time
}

func NewBroker() *Broker {
	return &Broker{
		clients: make(map[int64][]*Client),
		history: make(map[int64]*userHistory),
		nextID:  1,
		now:     time.Now,
	}
}

// Subscribe はクライアントを登録し、lastEventID より新しい未配信イベントを返す。
// lastEventID < 0 は初回接続（リプレイなし）を意味する。
//
// 注意: lastEventID が履歴の最古 ID より小さい場合（100件超の切断）は、
// 履歴に残っている範囲のみリプレイされる。それ以前のイベントは DB の通知一覧で確認できる。
func (b *Broker) Subscribe(userID int64, lastEventID int) (*Client, []Event, error) {
	b.mu.Lock()
	if len(b.clients[userID]) >= maxSSEConnectionsPerUser {
		b.mu.Unlock()
		return nil, nil, fmt.Errorf("too many SSE connections for user %d (limit: %d)", userID, maxSSEConnectionsPerUser)
	}
	c := &Client{ch: make(chan Event, 32)}
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
			var hist []Event
			if h := b.history[userID]; h != nil {
				hist = h.events
			}
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
	metrics.Global.IncSSEConnections()
	return c, missed, nil
}

func (b *Broker) Unsubscribe(userID int64, c *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// 切断のたびに掃除の機会を作る（接続が無くなったユーザーの履歴を捨てられるのは
	// ここから先なので、切断は掃除を回す自然なきっかけになる）。
	defer b.sweepHistoryLocked(b.now())
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
	metrics.Global.DecSSEConnections()
}

// EventNotificationsChanged は「このユーザーの通知の状態が変わった」ことだけを運ぶ
// SSE のイベント名。ベルの数字（未読通知数）は載せない。
//
// 以前は sync イベントで未読数そのものを配っていた。やめた理由は room_changed と同じで、
// 「サーバは事実だけを配り、派生値は必要になった画面が自分で取り直す」に揃えるため。
// 数を配ると (a) 既読処理のたびに COUNT が要る、(b) 数えられなかったときに送る値が
// 無い（handler では 0 を送っていた＝嘘をつく）、(c) 配った時点と画面が使う時点の
// ズレが残る。事実だけなら3つとも消える。
//
// 名前を sync から変えたのは、ペイロードから unreadCount が消えるため
// （unread_room → room_changed と同じ判断。名前を据え置くと、古いクライアントが
// undefined を数字として扱って壊れた表示になる）。
const EventNotificationsChanged = "notifications_changed"

// PublishNotificationsChangedToUser は通知の状態が変わったことをオンライン中の
// クライアントへ知らせる。
//
// 履歴に積まないのは、これが「取り直せ」という合図でしかないため。切断中に起きたぶんは、
// 再接続したクライアントがどのみち未読数を取り直すので、後から配る意味が無い。
func (b *Broker) PublishNotificationsChangedToUser(userID int64) {
	ev := Event{
		Type: EventNotificationsChanged,
		// 中身は無いが、空オブジェクトを送る（data 行そのものが無いと、
		// クライアント側の JSON.parse が空文字で失敗する）。
		Data: map[string]any{},
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
	now := b.now()
	h := b.history[userID]
	if h == nil {
		h = &userHistory{}
		b.history[userID] = h
	}
	h.events = append(h.events, ev)
	if len(h.events) > historySize {
		h.events = h.events[len(h.events)-historySize:]
	}
	h.updatedAt = now
	b.sweepHistoryLocked(now)

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

// sweepHistoryLocked は「もう誰も取りに来ない履歴」を捨てる。b.mu を持った状態で呼ぶこと。
//
// 捨てる条件は2つとも満たすとき:
//   - そのユーザーの接続が1本も無い（接続中なら、そのまま配信される／再接続の途中）
//   - 最後にイベントを書き足してから historyTTL を過ぎている
//
// 接続中のユーザーの履歴は残すが、こちらは1ユーザー historySize 件・接続数にも
// 上限（maxSSEConnectionsPerUser）があるので、増え続けることはない。
// 増え続けていたのは「接続していないユーザーのキーを一度も消していなかった」部分。
func (b *Broker) sweepHistoryLocked(now time.Time) {
	if now.Sub(b.lastSweep) < historySweepInterval {
		return
	}
	b.lastSweep = now
	for userID, h := range b.history {
		if len(b.clients[userID]) > 0 {
			continue
		}
		if now.Sub(h.updatedAt) >= historyTTL {
			delete(b.history, userID)
		}
	}
}

// ConnectedUserIDs はいま SSE を張っている利用者のIDを返す。
//
// 全員宛の配信（お知らせ）で「誰に送るか」を決めるために使う。以前はお知らせの
// 作成が全アクティブユーザーぶんのイベントを PublishToUser で流しており、接続して
// いない人のぶんまで履歴（history）に積まれていた。履歴の用途は「切断中のぶんを
// 再接続時にリプレイする」ことだけなので、一度も繋いでいない人の履歴は
// historyTTL の間ただメモリを占める。宛先を接続中に絞ればその山ごと消える。
//
// 返すのは呼んだ時点のスナップショット。返した直後に切れた人が混ざりうるが、
// 配信は「送れなければ捨てる」（select の default 節）ので害は無い。
func (b *Broker) ConnectedUserIDs() []int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]int64, 0, len(b.clients))
	for userID := range b.clients {
		ids = append(ids, userID)
	}
	return ids
}
