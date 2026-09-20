package sse

import (
	"context"
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

// Broker はユーザーごとの SSE クライアント接続を管理し、イベントを配信する。
type Broker struct {
	mu sync.Mutex
	// clients はこの台に繋がっているクライアント。ここだけは台ごとに持つ
	// （SSE の接続はどこか1台に貼り付くので、共有しようがない）。
	clients map[int64][]*Client

	// store は採番と履歴の置き場（Store のコメント参照）。
	store Store
	// fanout はイベントを全ての台へ配る口。台が1つなら自分へ返すだけ。
	fanout Fanout
}

// NewBroker は台が1つの構成向けの Broker を返す（採番も履歴も配信もプロセス内）。
func NewBroker() *Broker {
	return NewBrokerWithStore(NewMemoryStore(), nil)
}

// SetFanout は配信口を後から挿す。
//
// 台をまたぐ配信口は「受け取ったものをこの Broker へ渡す」ために Broker を
// 必要とし、Broker は配る先としてその配信口を必要とする。先に Broker を作って
// から挿せるようにしてあるのはそのため。サーバーの組み立て時に一度だけ呼ぶ。
func (b *Broker) SetFanout(f Fanout) {
	if f == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fanout = f
}

// NewBrokerWithStore は採番・履歴の置き場と、台またぎの配信口を指定して Broker を作る。
//
// fanout が nil なら、配信はこの台のクライアントにだけ届く。台を増やす構成では
// Redis の実装（infra/redis）を渡すこと。渡さないと、通知を作った台に繋がって
// いない利用者にはベルが光らない（エラーにはならないので気づきにくい）。
func NewBrokerWithStore(store Store, fanout Fanout) *Broker {
	if store == nil {
		store = NewMemoryStore()
	}
	b := &Broker{clients: make(map[int64][]*Client), store: store}
	if fanout == nil {
		// 自分へ返すだけの配線。Publish 側の分岐を無くすために、台が1つでも
		// 必ず fanout を通す（通さない近道を作ると、そちらだけ直し忘れる）。
		fanout = localFanout{b}
	}
	b.fanout = fanout
	return b
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
	b.clients[userID] = append(b.clients[userID], c)
	b.mu.Unlock()

	// リプレイは置き場に聞く。台をまたぐ構成では、別の台で起きたイベントも
	// ここから戻ってくる（自分の台のメモリだけを見ていると「履歴が無い」になる）。
	missed, err := b.store.Replay(context.Background(), userID, lastEventID)
	if err != nil {
		// リプレイできなくても接続は張る。通知の本体は DB にあるので、
		// クライアントは接続後に一覧を取り直せる。
		logger.Log.Error().Err(err).
			Int64("userID", userID).
			Int("lastEventID", lastEventID).
			Msg("failed to replay missed SSE events; connecting without replay")
		missed = nil
	}

	metrics.Global.IncSSEConnections()
	return c, missed, nil
}

func (b *Broker) Unsubscribe(userID int64, c *Client) {
	b.mu.Lock()
	// 切断のたびに掃除の機会を作る（接続が無くなったユーザーの履歴を捨てられるのは
	// ここから先なので、切断は掃除を回す自然なきっかけになる）。
	defer b.releaseIdleHistory()
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
	b.publish(Envelope{UserID: userID, Event: ev})
}

// Broadcast は現在接続中の全ユーザーにイベントを送信する。
// terms_updated など全員対象のシステムイベントに使う。履歴には記録しない。
func (b *Broker) Broadcast(eventType string, data map[string]any) {
	// UserID 0 は「全員へ」。履歴には積まないので採番もしない。
	b.publish(Envelope{UserID: 0, Event: Event{Type: eventType, Data: data}})
}

func (b *Broker) PublishToUser(userID int64, eventType string, data map[string]any) {
	// 採番と履歴は置き場に任せる。台をまたぐ構成では、同じ利用者のタブが
	// 別々の台に繋がっても連番が1本に保たれる（それぞれの台が1から数えると、
	// 同じIDの別イベントができて Last-Event-ID からのリプレイが狂う）。
	ev, err := b.store.Append(context.Background(), userID, eventType, data)
	if err != nil {
		// 採番できなければ履歴も残らない。配るのはやめる（ID 0 のまま配ると、
		// 受け取ったクライアントの Last-Event-ID が巻き戻る）。通知の本体は
		// DB にあるので、クライアントが次に一覧を取り直せば追いつく。
		logger.Log.Error().Err(err).
			Int64("userID", userID).
			Str("eventType", eventType).
			Msg("failed to record an SSE event; not delivering it")
		return
	}

	b.publish(Envelope{UserID: userID, Event: ev})
	b.releaseIdleHistory()
}

// publish はイベントを配信口へ渡す。台が1つなら自分のクライアントへ、
// 台が複数なら全ての台を経由して各台のクライアントへ届く。
func (b *Broker) publish(env Envelope) {
	b.mu.Lock()
	fanout := b.fanout
	b.mu.Unlock()

	if err := fanout.Publish(context.Background(), env); err != nil {
		logger.Log.Error().Err(err).
			Int64("userID", env.UserID).
			Str("eventType", env.Event.Type).
			Msg("failed to fan out an SSE event")
	}
}

// DeliverLocal はこの台に繋がっているクライアントへイベントを渡す。
//
// 配信口（Fanout）から呼ばれる。台をまたぐ実装では、他の台で起きたイベントも
// ここへ入ってくる。UserID 0 は全員宛。
func (b *Broker) DeliverLocal(env Envelope) {
	b.mu.Lock()
	var targets []*Client
	if env.UserID == 0 {
		for _, clients := range b.clients {
			targets = append(targets, clients...)
		}
	} else {
		targets = make([]*Client, len(b.clients[env.UserID]))
		copy(targets, b.clients[env.UserID])
	}
	b.mu.Unlock()

	for _, c := range targets {
		select {
		case c.ch <- env.Event:
		default:
			// チャンネルバッファ満杯。履歴に積んだイベント（ID>0）は
			// 次回再接続時にリプレイされる。
			logger.Log.Warn().
				Int64("userID", env.UserID).
				Int("eventID", env.Event.ID).
				Str("eventType", env.Event.Type).
				Msg("SSE channel buffer full: event dropped for active client, will replay on reconnect")
		}
	}
}

// releaseIdleHistory は「接続が1本も無くなった利用者」の履歴を捨てる機会を作る。
// 接続の有無はこの台しか知らないので、判断材料として渡す。
//
// 台をまたぐ構成では「この台に繋がっていない」だけでは捨ててよい根拠にならない
// （別の台に繋がっているかもしれない）。Redis の実装は寿命で消すので、
// 渡された判定を使わない。
func (b *Broker) releaseIdleHistory() {
	b.store.ReleaseIfIdle(context.Background(), func(userID int64) bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.clients[userID]) > 0
	})
}

func (b *Broker) ConnectedUserIDs() []int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]int64, 0, len(b.clients))
	for userID := range b.clients {
		ids = append(ids, userID)
	}
	return ids
}
