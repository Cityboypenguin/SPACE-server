package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// 配信アダプタ（chat.EventPublisher の実装）のテスト。
//
// 以前は *Resolver をまるごと保持していたため、ここを試すには巨大な Resolver を
// 組み立てるしかなく事実上テスト不能だった。必要な依存だけを受け取る形にしたので、
// 小さな fake を並べるだけで「どの経路へ配信したか」を確かめられる。

type fakePubSub struct {
	published map[string][]any
}

func newFakePubSub() *fakePubSub { return &fakePubSub{published: map[string][]any{}} }

func (f *fakePubSub) Publish(topic string, data interface{}) {
	f.published[topic] = append(f.published[topic], data)
}

type sseEvent struct {
	userID    int64
	eventType string
	data      map[string]any
}

type fakeSSEBroker struct {
	events []sseEvent
	syncs  map[int64]int
}

func newFakeSSEBroker() *fakeSSEBroker { return &fakeSSEBroker{syncs: map[int64]int{}} }

func (f *fakeSSEBroker) PublishToUser(userID int64, eventType string, data map[string]any) {
	f.events = append(f.events, sseEvent{userID: userID, eventType: eventType, data: data})
}

func (f *fakeSSEBroker) PublishSyncToUser(userID int64, unreadCount int) {
	f.syncs[userID] = unreadCount
}

type fakeNotificationPublisher struct {
	published []notificationuc.PublishParams
	batched   []notificationuc.PublishParams
	err       error
}

func (f *fakeNotificationPublisher) Publish(_ context.Context, params notificationuc.PublishParams) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, params)
	return nil
}

func (f *fakeNotificationPublisher) PublishBatch(_ context.Context, params []notificationuc.PublishParams) error {
	if f.err != nil {
		return f.err
	}
	f.batched = append(f.batched, params...)
	return nil
}

// fakeUnreadCounts は未読SSEの宛先。授業ルーム用とそれ以外用の2経路があるので、
// どちらが呼ばれたかを calls で見分ける。
type fakeUnreadCounts struct {
	counts map[int64]int
	err    error
	calls  int
}

func (f *fakeUnreadCounts) Execute(_ context.Context, _ int64, _ int64) (map[int64]int, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.counts, nil
}

type fakeGetMessageByID struct{ msg *model.Message }

func (f *fakeGetMessageByID) Execute(_ context.Context, _ int64) (*model.Message, error) {
	return f.msg, nil
}

type fakeGetAnonymousIdentity struct{ identity *model.RoomAnonymousIdentity }

func (f *fakeGetAnonymousIdentity) Execute(_ context.Context, _, _ int64) (*model.RoomAnonymousIdentity, error) {
	return f.identity, nil
}

type fakeMarkAllAsReadByActor struct{ calls int }

func (f *fakeMarkAllAsReadByActor) Execute(_ context.Context, _ int64, _ string, _ int64) error {
	f.calls++
	return nil
}

type fakeCountUnreadNotifications struct{ count int }

func (f *fakeCountUnreadNotifications) Execute(_ context.Context, _ int64) (int, error) {
	return f.count, nil
}

type publisherHarness struct {
	pubsub       *fakePubSub
	sse          *fakeSSEBroker
	notify       *fakeNotificationPublisher
	memberUnread *fakeUnreadCounts
	courseUnread *fakeUnreadCounts
	publisher    chatusecase.EventPublisher
}

func newPublisherHarness() *publisherHarness {
	h := &publisherHarness{
		pubsub:       newFakePubSub(),
		sse:          newFakeSSEBroker(),
		notify:       &fakeNotificationPublisher{},
		memberUnread: &fakeUnreadCounts{counts: map[int64]int{11: 2}},
		courseUnread: &fakeUnreadCounts{counts: map[int64]int{21: 5}},
	}
	h.publisher = NewChatEventPublisher(ChatEventPublisherDeps{
		PubSub:                         h.pubsub,
		SSEBroker:                      h.sse,
		NotificationPublisher:          h.notify,
		MembersUnreadCounts:            h.memberUnread,
		CourseRoomUnreadCounts:         h.courseUnread,
		GetMessage:                     &fakeGetMessageByID{},
		GetAnonymousIdentity:           &fakeGetAnonymousIdentity{},
		MarkNotificationsAsReadByActor: &fakeMarkAllAsReadByActor{},
		CountUnreadNotifications:       &fakeCountUnreadNotifications{count: 4},
	})
	return h
}

// 授業ルームは room_users を使わないので、未読SSEの宛先も専用の経路で引く。
// ここを取り違えると履修者に未読が1件も届かない。
func TestChatEventPublisher_MessageSent_CourseRoomUsesTheCourseUnreadRoute(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 1, Type: model.RoomTypeCourse}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:    room,
		Message: &model.Message{ID: 100, RoomID: 1, UserID: 10, Content: "hi"},
		ActorID: 10,
	})

	if h.courseUnread.calls != 1 || h.memberUnread.calls != 0 {
		t.Fatalf("course route calls = %d, room_users route calls = %d, want 1 and 0",
			h.courseUnread.calls, h.memberUnread.calls)
	}

	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":message:added"]); got != 1 {
		t.Fatalf("message:added publishes = %d, want 1", got)
	}
	if len(h.sse.events) != 1 || h.sse.events[0].userID != 21 {
		t.Fatalf("unread SSE events = %+v, want one for the enrolled user 21", h.sse.events)
	}
	// 授業内チャットは匿名。DM 通知を出す相手も居ない。
	if len(h.notify.published) != 0 {
		t.Errorf("notifications = %+v, want none in a course room", h.notify.published)
	}
}

// DM は room_users 側の未読経路を使い、相手にだけ通知を出す（自分には出さない）。
func TestChatEventPublisher_MessageSent_DMNotifiesThePartnerOnly(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 3, Type: model.RoomTypeDM}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:      room,
		Message:   &model.Message{ID: 100, RoomID: 3, UserID: 10, Content: "hi"},
		ActorID:   10,
		MemberIDs: []int64{10, 11},
	})

	if h.memberUnread.calls != 1 || h.courseUnread.calls != 0 {
		t.Fatalf("room_users route calls = %d, course route calls = %d, want 1 and 0",
			h.memberUnread.calls, h.courseUnread.calls)
	}
	if len(h.notify.published) != 1 {
		t.Fatalf("dm notifications = %d, want 1 (相手のぶんだけ)", len(h.notify.published))
	}
	notified := h.notify.published[0]
	if notified.UserID != 11 {
		t.Errorf("notified user = %d, want 11 (送信者自身には送らない)", notified.UserID)
	}
	if notified.Type != notificationuc.TypeDM || notified.Message != "hi" {
		t.Errorf("notification = %+v, want a dm notification carrying the preview", notified)
	}
}

// 通知はベストエフォート。落ちても購読配信と未読SSEは止めない
// （メッセージは保存済みなので、通知の失敗で画面反映まで諦めるのは筋が悪い）。
func TestChatEventPublisher_MessageSent_NotificationFailureDoesNotStopDelivery(t *testing.T) {
	h := newPublisherHarness()
	h.notify.err = errors.New("notification backend is down")
	room := &model.Room{ID: 3, Type: model.RoomTypeDM}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:      room,
		Message:   &model.Message{ID: 100, RoomID: 3, UserID: 10, Content: "hi"},
		ActorID:   10,
		MemberIDs: []int64{10, 11},
	})

	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":message:added"]); got != 1 {
		t.Fatalf("message:added publishes = %d, want 1 even when the notification fails", got)
	}
	if len(h.sse.events) != 1 {
		t.Fatalf("unread SSE events = %d, want 1 even when the notification fails", len(h.sse.events))
	}
}

// 未読件数が引けないときは未読SSEだけを諦める。購読配信と通知は通す。
func TestChatEventPublisher_MessageSent_UnreadCountFailureSkipsOnlyTheBadge(t *testing.T) {
	h := newPublisherHarness()
	h.memberUnread.err = errors.New("db is down")
	room := &model.Room{ID: 3, Type: model.RoomTypeDM}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:      room,
		Message:   &model.Message{ID: 100, RoomID: 3, UserID: 10, Content: "hi"},
		ActorID:   10,
		MemberIDs: []int64{10, 11},
	})

	if len(h.sse.events) != 0 {
		t.Errorf("unread SSE events = %+v, want none when the counts cannot be loaded", h.sse.events)
	}
	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":message:added"]); got != 1 {
		t.Errorf("message:added publishes = %d, want 1", got)
	}
	if len(h.notify.published) != 1 {
		t.Errorf("dm notifications = %d, want 1", len(h.notify.published))
	}
}

// 既読の配信は相手側の既読表示のためのもの。授業内チャットは匿名なので、
// 誰が読んだか（実ユーザーID）をルームの購読者へ流さない。
func TestChatEventPublisher_RoomMarkedAsRead_CourseRoomDoesNotBroadcastTheReader(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 1, Type: model.RoomTypeCourse}

	h.publisher.RoomMarkedAsRead(context.Background(), chatusecase.RoomMarkedAsReadEvent{
		Room:    room,
		ActorID: 10,
	})

	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":read_status"]); got != 0 {
		t.Errorf("read_status publishes = %d, want 0 in an anonymous course room", got)
	}
	// 自分の未読バッジ（0件）の反映だけは送る。
	if len(h.sse.events) != 1 || h.sse.events[0].userID != 10 {
		t.Fatalf("unread SSE events = %+v, want one for the reader", h.sse.events)
	}
}

func TestChatEventPublisher_RoomMarkedAsRead_DMSyncsTheBell(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 3, Type: model.RoomTypeDM}

	h.publisher.RoomMarkedAsRead(context.Background(), chatusecase.RoomMarkedAsReadEvent{
		Room:      room,
		ActorID:   10,
		MemberIDs: []int64{10, 11},
	})

	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":read_status"]); got != 1 {
		t.Errorf("read_status publishes = %d, want 1 so the partner sees the read receipt", got)
	}
	if got, ok := h.sse.syncs[10]; !ok || got != 4 {
		t.Errorf("bell sync for the reader = %d (present=%v), want 4", got, ok)
	}
}
