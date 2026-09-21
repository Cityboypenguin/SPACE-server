package sse

import (
	"context"
	"sync"
	"testing"
	"time"
)

// drain は Subscribe 直後にチャンネルへ既に積まれているイベントは無い前提で、
// PublishToUser 経由で流れてくるイベントを1件だけ非ブロッキングで取り出す。
func tryRecv(c *Client) (Event, bool) {
	select {
	case ev := <-c.ch:
		return ev, true
	default:
		return Event{}, false
	}
}

// 初回接続(lastEventID=-1)ではリプレイされないこと。
func TestSubscribe_FirstConnect_NoReplay(t *testing.T) {
	b := NewBroker()
	b.PublishToUser(1, "notification", map[string]any{"msg": "before"})

	_, missed, err := b.Subscribe(1, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missed) != 0 {
		t.Fatalf("first connect should not replay, got %d events", len(missed))
	}
}

// 切断中に発生したイベントが、再接続(Last-Event-ID)でリプレイされること。
// B の「スリープ/タブ復帰で取りこぼしを回収する」挙動の土台。
func TestSubscribe_Reconnect_ReplaysMissed(t *testing.T) {
	b := NewBroker()

	// 接続 → イベント1件受信 → 切断
	c1, _, _ := b.Subscribe(1, -1)
	b.PublishToUser(1, "notification", map[string]any{"n": 1})
	ev1, ok := tryRecv(c1)
	if !ok {
		t.Fatal("expected to receive event 1 while connected")
	}
	b.Unsubscribe(1, c1)

	// 切断中に2件発生
	b.PublishToUser(1, "notification", map[string]any{"n": 2})
	b.PublishToUser(1, "notification", map[string]any{"n": 3})

	// ev1.ID を Last-Event-ID として再接続 → 2,3 がリプレイされる
	_, missed, err := b.Subscribe(1, ev1.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missed) != 2 {
		t.Fatalf("expected 2 replayed events, got %d", len(missed))
	}
	if missed[0].Data["n"] != 2 || missed[1].Data["n"] != 3 {
		t.Fatalf("unexpected replay contents: %+v", missed)
	}
}

// 履歴上限(historySize)を超えた古いイベントは evict され、リプレイ対象外になること。
func TestSubscribe_Reconnect_HistoryEviction(t *testing.T) {
	b := NewBroker()
	total := historySize + 10
	for i := 0; i < total; i++ {
		b.PublishToUser(1, "notification", map[string]any{"i": i})
	}
	// lastEventID=0（=何も受け取っていない）で再接続しても、保持されるのは直近 historySize 件のみ
	_, missed, _ := b.Subscribe(1, 0)
	if len(missed) != historySize {
		t.Fatalf("expected replay capped at %d, got %d", historySize, len(missed))
	}
}

// 1ユーザーの同時接続数が上限に達したら Subscribe がエラーを返すこと。
func TestSubscribe_MaxConnectionsPerUser(t *testing.T) {
	b := NewBroker()
	for i := 0; i < maxSSEConnectionsPerUser; i++ {
		if _, _, err := b.Subscribe(1, -1); err != nil {
			t.Fatalf("connection %d should succeed, got %v", i, err)
		}
	}
	if _, _, err := b.Subscribe(1, -1); err == nil {
		t.Fatal("expected error when exceeding max connections per user")
	}
}

// newTestBroker は時計を手で進められる Broker を返す（履歴の期限切れを待たずに試すため）。
//
// 採番と履歴は Store 側にあるので、差し替えるのはそちらの時計。
func newTestBroker(now *time.Time) *Broker {
	store := NewMemoryStore()
	store.now = func() time.Time { return *now }
	return NewBrokerWithStore(store, nil)
}

// testStore は newTestBroker が挿した置き場を取り出す（履歴の中身を覗くため）。
func testStore(b *Broker) *MemoryStore { return b.store.(*MemoryStore) }

func historyLen(b *Broker) int {
	s := testStore(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.history)
}

// 接続が無いユーザーの履歴が、期限切れのあと捨てられること。
// 以前はユーザーのキーを一度も消しておらず、ユーザー数に比例してメモリが増え続けていた。
func TestHistory_EvictsDisconnectedUsersAfterTTL(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	b := newTestBroker(&now)

	// 誰も繋いでいない100人ぶんの履歴を作る
	for id := int64(1); id <= 100; id++ {
		b.PublishToUser(id, "notification", map[string]any{"id": id})
	}
	if got := historyLen(b); got != 100 {
		t.Fatalf("expected 100 users in history, got %d", got)
	}

	// 期限切れのあと、何かのきっかけ（ここでは別ユーザーへの配信）で掃除が走る
	now = now.Add(historyTTL + time.Minute)
	b.PublishToUser(999, "notification", map[string]any{"id": 999})

	if got := historyLen(b); got != 1 {
		t.Fatalf("expected the stale histories to be released, got %d users", got)
	}
}

// 切断がきっかけでも掃除が走ること（配信が止まっていても解放される）。
func TestHistory_EvictedOnUnsubscribe(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	b := newTestBroker(&now)

	b.PublishToUser(1, "notification", map[string]any{"n": 1})
	c, _, _ := b.Subscribe(2, -1)
	b.PublishToUser(2, "notification", map[string]any{"n": 2})

	now = now.Add(historyTTL + time.Minute)
	b.Unsubscribe(2, c) // ここで両者とも接続なし・期限切れ

	if got := historyLen(b); got != 0 {
		t.Fatalf("expected every stale history to be released on unsubscribe, got %d users", got)
	}
}

// 期限内であれば履歴は残り、再接続でリプレイできること（掃除がリプレイを壊さない）。
func TestHistory_KeptWithinTTLForReconnect(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	b := newTestBroker(&now)

	c1, _, _ := b.Subscribe(1, -1)
	b.PublishToUser(1, "notification", map[string]any{"n": 1})
	ev1, _ := tryRecv(c1)
	b.Unsubscribe(1, c1)

	b.PublishToUser(1, "notification", map[string]any{"n": 2})

	// 期限内の再接続（掃除は走るが、このユーザーの履歴は消えない）
	now = now.Add(historyTTL - time.Minute)
	b.PublishToUser(2, "notification", map[string]any{"other": true})

	_, missed, err := b.Subscribe(1, ev1.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missed) != 1 || missed[0].Data["n"] != 2 {
		t.Fatalf("expected the missed event to still be replayable, got %+v", missed)
	}
}

// 接続し続けているユーザーの履歴は、最後の配信から期限を過ぎても消さないこと
// （繋ぎっぱなしのまま Last-Event-ID 付きで張り直される可能性が残るため）。
// 接続中ぶんのメモリは1ユーザー historySize 件・接続数上限で抑えられている。
func TestHistory_KeptWhileConnected(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	b := newTestBroker(&now)

	if _, _, err := b.Subscribe(1, -1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b.PublishToUser(1, "notification", map[string]any{"n": 1})

	now = now.Add(historyTTL + time.Minute)
	b.PublishToUser(2, "notification", map[string]any{"n": 2}) // 掃除のきっかけ

	st := testStore(b)
	st.mu.Lock()
	_, kept := st.history[1]
	st.mu.Unlock()
	if !kept {
		t.Fatal("a connected user's history must not be released")
	}
}

// 通知の状態変化は、数字を載せずにオンライン中のクライアントへ届くこと。
// 履歴には積まない（切断中に起きたぶんは、再接続したクライアントが取り直す）。
func TestPublishNotificationsChangedToUser_SendsAFactWithoutNumbers(t *testing.T) {
	b := NewBroker()
	c, _, err := b.Subscribe(1, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b.PublishNotificationsChangedToUser(1)

	ev, ok := tryRecv(c)
	if !ok {
		t.Fatal("expected the connected client to receive the event")
	}
	if ev.Type != EventNotificationsChanged {
		t.Fatalf("event type = %q, want %q", ev.Type, EventNotificationsChanged)
	}
	if len(ev.Data) != 0 {
		t.Fatalf("expected no derived values in the payload, got %+v", ev.Data)
	}
	if ev.ID != 0 {
		t.Fatalf("expected no event id (not replayable), got %d", ev.ID)
	}
	if historyLen(b) != 0 {
		t.Fatal("the fact-only event must not be kept in the replay history")
	}
}

// イベントIDはユーザーごとの連番であること。
//
// 以前は全ユーザー共通の連番だったので、他人宛の配信が挟まるたびに自分宛のID列が
// 飛んだ。受け手は Last-Event-ID しか持てないため、欠番を「取りこぼし」と区別できない。
func TestPublishToUser_EventIDsAreSequentialPerUser(t *testing.T) {
	b := NewBroker()

	c1, _, _ := b.Subscribe(1, -1)
	c2, _, _ := b.Subscribe(2, -1)

	// 交互に配る。共通カウンタなら 1 の列は 1,3,5、2 の列は 2,4,6 になる。
	for i := 0; i < 3; i++ {
		b.PublishToUser(1, "notification", map[string]any{"n": i})
		b.PublishToUser(2, "notification", map[string]any{"n": i})
	}

	for _, tc := range []struct {
		userID int64
		c      *Client
	}{{1, c1}, {2, c2}} {
		for want := 1; want <= 3; want++ {
			ev, ok := tryRecv(tc.c)
			if !ok {
				t.Fatalf("user %d: expected event %d", tc.userID, want)
			}
			if ev.ID != want {
				t.Fatalf("user %d: event id = %d, want %d (IDs must not skip because of other users' events)",
					tc.userID, ev.ID, want)
			}
		}
	}
}

// 他人宛の配信が挟まっても、自分のリプレイ範囲が変わらないこと。
// 共通カウンタのころは「履歴の最古IDが lastEventID+1 より大きい」という
// 取りこぼし判定が、他人宛に消費された欠番だけで誤爆していた。
func TestSubscribe_ReplayUnaffectedByOtherUsersEvents(t *testing.T) {
	b := NewBroker()

	c1, _, _ := b.Subscribe(1, -1)
	b.PublishToUser(1, "notification", map[string]any{"n": 1})
	ev1, ok := tryRecv(c1)
	if !ok {
		t.Fatal("expected to receive event 1 while connected")
	}
	if ev1.ID != 1 {
		t.Fatalf("first event id = %d, want 1", ev1.ID)
	}
	b.Unsubscribe(1, c1)

	// 切断中、大量に他人宛の配信が走る。
	for i := 0; i < 50; i++ {
		b.PublishToUser(int64(100+i), "notification", map[string]any{"i": i})
	}
	// 自分宛は2件だけ。
	b.PublishToUser(1, "notification", map[string]any{"n": 2})
	b.PublishToUser(1, "notification", map[string]any{"n": 3})

	_, missed, err := b.Subscribe(1, ev1.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missed) != 2 {
		t.Fatalf("expected exactly the 2 events addressed to this user, got %d: %+v", len(missed), missed)
	}
	if missed[0].ID != 2 || missed[1].ID != 3 {
		t.Fatalf("replayed ids = %d,%d, want 2,3", missed[0].ID, missed[1].ID)
	}
	if missed[0].Data["n"] != 2 || missed[1].Data["n"] != 3 {
		t.Fatalf("unexpected replay contents: %+v", missed)
	}
}

// 履歴が TTL で掃除された後に再接続しても、採番が 1 からやり直されるだけで
// 落ちないこと（リプレイできるものは無い＝missed は空）。
func TestSubscribe_AfterHistorySweep_RestartsNumbering(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	b := newTestBroker(&now)

	c1, _, _ := b.Subscribe(1, -1)
	b.PublishToUser(1, "notification", map[string]any{"n": 1})
	ev1, _ := tryRecv(c1)
	b.Unsubscribe(1, c1)

	now = now.Add(historyTTL + time.Minute)
	b.PublishToUser(999, "notification", map[string]any{"sweep": true}) // 掃除のきっかけ
	if historyLen(b) != 1 {
		t.Fatalf("expected the stale history to be released, got %d users", historyLen(b))
	}

	c2, missed, err := b.Subscribe(1, ev1.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missed) != 0 {
		t.Fatalf("nothing can be replayed once the history is gone, got %+v", missed)
	}
	b.PublishToUser(1, "notification", map[string]any{"n": 2})
	ev2, ok := tryRecv(c2)
	if !ok {
		t.Fatal("expected the new event")
	}
	if ev2.ID != 1 {
		t.Fatalf("event id = %d, want 1 (numbering restarts with the history)", ev2.ID)
	}
}

// --- 台をまたいだ配信（項目9） --------------------------------------------

// sharedFanout は複数の Broker を1本の配信口へ繋いだもの。
// 「同じ Redis を見ている複数の台」に相当する。
type sharedFanout struct {
	mu        sync.Mutex
	brokers   []*Broker
	published int
}

func (f *sharedFanout) attach(b *Broker) {
	f.mu.Lock()
	f.brokers = append(f.brokers, b)
	f.mu.Unlock()
	b.SetFanout(f)
}

func (f *sharedFanout) Publish(_ context.Context, env Envelope) error {
	f.mu.Lock()
	targets := make([]*Broker, len(f.brokers))
	copy(targets, f.brokers)
	f.published++
	f.mu.Unlock()

	for _, b := range targets {
		b.DeliverLocal(env)
	}
	return nil
}

// newClusteredBrokers は同じ置き場・同じ配信口を共有する台を n 台作る。
func newClusteredBrokers(n int) []*Broker {
	store := NewMemoryStore()
	fanout := &sharedFanout{}
	brokers := make([]*Broker, n)
	for i := range brokers {
		b := NewBrokerWithStore(store, nil)
		fanout.attach(b)
		brokers[i] = b
	}
	return brokers
}

// TestPublishToUser_ReachesClientsOnOtherInstances は項目9の本体。
//
// SSE の接続はどこか1台に貼り付くが、通知を作る操作は別の台に当たりうる。
// 配信が自分の台のクライアントにしか届かないと、通知を作った台に繋がって
// いない利用者にはベルが光らない。エラーにはならないので、1台で動かして
// いる間は誰も気づけない。
func TestPublishToUser_ReachesClientsOnOtherInstances(t *testing.T) {
	instances := newClusteredBrokers(2)
	instanceA, instanceB := instances[0], instances[1]

	// 利用者は2台目に繋がっている。
	client, _, err := instanceB.Subscribe(1, -1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// 通知は1台目で作られた。
	instanceA.PublishToUser(1, "notification", map[string]any{"n": 1})

	select {
	case ev := <-client.ch:
		if ev.Type != "notification" {
			t.Fatalf("type = %q, want notification", ev.Type)
		}
		if ev.Data["n"] != 1 {
			t.Fatalf("data = %+v", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("別の台で起きた通知が届かない")
	}
}

// TestBroadcast_ReachesClientsOnOtherInstances は全員宛の配信（規約更新など）。
func TestBroadcast_ReachesClientsOnOtherInstances(t *testing.T) {
	instances := newClusteredBrokers(2)
	instanceA, instanceB := instances[0], instances[1]

	clientOnA, _, err := instanceA.Subscribe(1, -1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	clientOnB, _, err := instanceB.Subscribe(2, -1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	instanceA.Broadcast("terms_updated", map[string]any{"version": "2.0"})

	for name, c := range map[string]*Client{"1台目": clientOnA, "2台目": clientOnB} {
		select {
		case ev := <-c.ch:
			if ev.Type != "terms_updated" {
				t.Fatalf("%s: type = %q", name, ev.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s に全員宛の配信が届かない", name)
		}
	}
}

// TestPublishToUser_EventIDsStayUniqueAcrossInstances は採番が台をまたいで
// 1本に保たれることを確かめる。
//
// それぞれの台が1から数えると、同じIDの別イベントができる。SSE の再接続は
// Last-Event-ID しか運べないので、リプレイの起点が狂って取りこぼしや
// 二重配信になる。
func TestPublishToUser_EventIDsStayUniqueAcrossInstances(t *testing.T) {
	instances := newClusteredBrokers(2)

	client, _, err := instances[0].Subscribe(1, -1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// 1台目→2台目→1台目 の順に発行する。
	for i, instance := range []*Broker{instances[0], instances[1], instances[0]} {
		instance.PublishToUser(1, "notification", map[string]any{"i": i})
	}

	seen := make(map[int]bool)
	for range 3 {
		select {
		case ev := <-client.ch:
			if seen[ev.ID] {
				t.Fatalf("ID %d が2回使われた（台ごとに採番している）", ev.ID)
			}
			seen[ev.ID] = true
		case <-time.After(time.Second):
			t.Fatal("配信が届かない")
		}
	}
	for id := 1; id <= 3; id++ {
		if !seen[id] {
			t.Fatalf("ID %d が抜けている: %v", id, seen)
		}
	}
}

// TestSubscribe_ReplaysEventsPublishedByAnotherInstance は、別の台で起きた
// イベントも再接続時にリプレイされることを確かめる。
//
// 履歴が台ごとだと、1台目で起きたイベントは2台目へ再接続したタブからは
// 「履歴が無い」ことになり、切断中のぶんが丸ごと抜ける。
func TestSubscribe_ReplaysEventsPublishedByAnotherInstance(t *testing.T) {
	instances := newClusteredBrokers(2)
	instanceA, instanceB := instances[0], instances[1]

	// 1台目に繋いで1件受け取り、切断する。
	client, _, err := instanceA.Subscribe(1, -1)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	instanceA.PublishToUser(1, "notification", map[string]any{"n": 1})
	first := <-client.ch
	instanceA.Unsubscribe(1, client)

	// 切断中に、別の台でイベントが起きる。
	instanceB.PublishToUser(1, "notification", map[string]any{"n": 2})

	// 2台目へ繋ぎ直すと、切断中のぶんが戻ってくる。
	_, missed, err := instanceB.Subscribe(1, first.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if len(missed) != 1 {
		t.Fatalf("replayed = %d 件, want 1（別の台で起きたぶんが抜けている）", len(missed))
	}
	if missed[0].Data["n"] != 2 {
		t.Fatalf("replayed = %+v", missed[0].Data)
	}
}

// 配信中に切断が重なってもプロセスが落ちないこと。
//
// 以前は宛先を写し取ってから錠を放し、その外で送っていた。写した直後に
// Unsubscribe がチャンネルを閉じると、閉じたチャンネルへの送信になって
// プロセスごと panic する。SSE は「タブを閉じた」「回線が切れた」で日常的に
// 切断が起きるので、賑やかな時間帯ほど当たりやすい。
//
// panic するのは配信した goroutine であって切断した側ではないため、落ちるのは
// 無関係な利用者のリクエストを処理している最中になる。
func TestBroker_DeliveringWhileClientsDisconnectDoesNotPanic(t *testing.T) {
	b := NewBroker()

	const userID = int64(1)
	var wg sync.WaitGroup

	// 配信し続ける側。1本だと切断側が錠を取りやすく、危ない隙間に当たりにくい。
	stop := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.DeliverLocal(Envelope{UserID: userID, Event: Event{Type: "notifications_changed"}})
					b.DeliverLocal(Envelope{UserID: 0, Event: Event{Type: "terms_updated"}})
				}
			}
		}()
	}

	// 繋いでは切る側。接続数の上限があるので、1本ずつ回す。
	for range 5000 {
		c, _, err := b.Subscribe(userID, -1)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		b.Unsubscribe(userID, c)
	}

	close(stop)
	wg.Wait()
}
