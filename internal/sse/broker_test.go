package sse

import "testing"

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
