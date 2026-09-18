package graph

import (
	"context"
	"testing"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
)

// recvWithin は転送ループが非同期なぶんだけ待って1件受け取る。
func recvWithin[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before delivering an event")
		}
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the event")
	}
	var zero T
	return zero
}

func expectNothing[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("expected no delivery, got %+v", v)
	case <-time.After(50 * time.Millisecond):
	}
}

// 各 subscription が使う型が、共通化後もそれぞれのトピックで届くこと。
// message / question / answer / poll は同じ転送ループを型引数だけ変えて使うので、
// ここで代表的な4型をまとめて確かめる。
func TestSubscribeTopic_DeliversEachSubscriptionPayload(t *testing.T) {
	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scope := subscriptionScope{UserID: 7, RoomID: "room-1"}

	messages := subscribeTopic[*gqlmodel.Message](ctx, ps, "room-1:message:added", scope)
	questions := subscribeTopic[*gqlmodel.Question](ctx, ps, "room-1:question:added", scope)
	answers := subscribeTopic[*gqlmodel.Answer](ctx, ps, "question-1:answer:added", scope)
	polls := subscribeTopic[*gqlmodel.Poll](ctx, ps, "room-1:poll:added", scope)

	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "m1"})
	ps.Publish("room-1:question:added", &gqlmodel.Question{ID: "q1"})
	ps.Publish("question-1:answer:added", &gqlmodel.Answer{ID: "a1"})
	ps.Publish("room-1:poll:added", &gqlmodel.Poll{ID: "p1"})

	if got := recvWithin(t, messages); got.ID != "m1" {
		t.Fatalf("message: got %q", got.ID)
	}
	if got := recvWithin(t, questions); got.ID != "q1" {
		t.Fatalf("question: got %q", got.ID)
	}
	if got := recvWithin(t, answers); got.ID != "a1" {
		t.Fatalf("answer: got %q", got.ID)
	}
	if got := recvWithin(t, polls); got.ID != "p1" {
		t.Fatalf("poll: got %q", got.ID)
	}
}

// 購読しているトピックへ別の型が流れてきても、ループが止まらず後続が届くこと。
func TestSubscribeTopic_IgnoresForeignPayloadTypes(t *testing.T) {
	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := subscribeTopic[*gqlmodel.Message](ctx, ps, "room-1:message:added", subscriptionScope{UserID: 1})

	ps.Publish("room-1:message:added", "not a message")
	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "m2"})

	if got := recvWithin(t, ch); got.ID != "m2" {
		t.Fatalf("expected the well-typed event, got %q", got.ID)
	}
}

// 既読の購読が使う絞り込み（自分のイベントは配らない）が効くこと。
func TestSubscribeTopicFunc_FiltersOutOwnReadStatus(t *testing.T) {
	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	me := "user-1"
	ch := subscribeTopicFunc(ctx, ps, "room-1:read_status", subscriptionScope{UserID: 1, RoomID: "room-1"},
		func(u *gqlmodel.RoomReadStatusUpdate) (*gqlmodel.RoomReadStatusUpdate, bool) {
			return u, u.UserID != me
		})

	ps.Publish("room-1:read_status", &gqlmodel.RoomReadStatusUpdate{UserID: me})
	expectNothing(t, ch)

	ps.Publish("room-1:read_status", &gqlmodel.RoomReadStatusUpdate{UserID: "user-2"})
	if got := recvWithin(t, ch); got.UserID != "user-2" {
		t.Fatalf("expected the other user's read status, got %q", got.UserID)
	}
}

// 取り込み状況の購読が使う変換（ドメインの値 → GraphQL 型）が効くこと。
func TestSubscribeTopicFunc_ConvertsDomainValue(t *testing.T) {
	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := subscribeTopicFunc(ctx, ps, CourseImportStatusTopic, subscriptionScope{UserID: 1},
		func(s courseimport.Status) (*gqlmodel.CourseImportStatus, bool) {
			return toGraphCourseImportStatus(s), true
		})

	ps.Publish(CourseImportStatusTopic, courseimport.Status{State: courseimport.StateRunning, Year: 2026})

	got := recvWithin(t, ch)
	if got.State != gqlmodel.CourseImportState(courseimport.StateRunning) {
		t.Fatalf("expected the converted status to keep the state, got %+v", got)
	}
	if got.Year == nil || *got.Year != 2026 {
		t.Fatalf("expected the converted status to keep the year, got %+v", got.Year)
	}
}

// context が終わったらチャンネルを閉じ、PubSub から購読を外すこと
// （外し忘れると配信のたびに誰も読まないチャンネルへ書き続ける）。
func TestSubscribeTopic_ClosesAndUnsubscribesOnContextDone(t *testing.T) {
	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())

	ch := subscribeTopic[*gqlmodel.Message](ctx, ps, "room-1:message:added", subscriptionScope{UserID: 1})
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected the subscription channel to be closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription channel was not closed after context cancellation")
	}

	// 購読が外れていれば、以降の配信は誰にも届かない（ここで panic しないことも含む）。
	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "m3"})
}
