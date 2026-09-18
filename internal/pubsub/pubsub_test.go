package pubsub

import "testing"

// 最後の購読者が抜けたらトピックそのものが消えること。
// トピック名はルームID・メッセージID・質問IDごとに作られるため、空になっても
// 残していると使われなくなったキーがプロセスの寿命ぶん積み上がる。
func TestUnsubscribe_RemovesTopicWhenLastSubscriberLeaves(t *testing.T) {
	ps := New()

	ch := ps.Subscribe("room-1:message:added")
	ps.Unsubscribe("room-1:message:added", ch)

	if _, ok := ps.subs["room-1:message:added"]; ok {
		t.Fatal("expected the topic to be removed once it had no subscribers")
	}
	if len(ps.subs) != 0 {
		t.Fatalf("expected no topics left, got %d", len(ps.subs))
	}
}

// 複数購読者のうち1人だけ抜けた場合はトピックが残り、残りには配信が届くこと。
func TestUnsubscribe_KeepsTopicWhileOtherSubscribersRemain(t *testing.T) {
	ps := New()

	first := ps.Subscribe("room-1:message:added")
	second := ps.Subscribe("room-1:message:added")

	ps.Unsubscribe("room-1:message:added", first)

	subs, ok := ps.subs["room-1:message:added"]
	if !ok {
		t.Fatal("expected the topic to remain while another subscriber is attached")
	}
	if len(subs) != 1 {
		t.Fatalf("expected exactly one remaining subscriber, got %d", len(subs))
	}

	ps.Publish("room-1:message:added", "hello")
	select {
	case got := <-second:
		if got != "hello" {
			t.Fatalf("remaining subscriber got %v", got)
		}
	default:
		t.Fatal("remaining subscriber did not receive the published value")
	}

	// 抜けた側のチャンネルは閉じられている（読み手が残り続けないように）。
	if _, open := <-first; open {
		t.Fatal("expected the unsubscribed channel to be closed")
	}

	// 2人目も抜ければトピックは消える。
	ps.Unsubscribe("room-1:message:added", second)
	if len(ps.subs) != 0 {
		t.Fatalf("expected no topics left, got %d", len(ps.subs))
	}
}
