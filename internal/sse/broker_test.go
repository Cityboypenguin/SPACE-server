package sse

import (
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
func newTestBroker(now *time.Time) *Broker {
	b := NewBroker()
	b.now = func() time.Time { return *now }
	return b
}

func historyLen(b *Broker) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.history)
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

	b.mu.Lock()
	_, kept := b.history[1]
	b.mu.Unlock()
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
