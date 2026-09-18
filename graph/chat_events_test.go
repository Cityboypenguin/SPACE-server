package graph

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// 配信アダプタ（chat.EventPublisher の実装）のテスト。
//
// 以前は *Resolver をまるごと保持していたため、ここを試すには巨大な Resolver を
// 組み立てるしかなく事実上テスト不能だった。必要な依存だけを受け取る形にしたので、
// 小さな fake を並べるだけで「どの経路へ配信したか」を確かめられる。
//
// 非同期にする部分はアダプタが deps.Async へ渡すので、テストでは「その場で実行する
// runner」を差し込み、全ての配信を同期に観測する。こうすると「何をどこへ配信するか」
// だけを素直に確かめられる。非同期化そのもの（ctx・panic・Wait）は
// usecase/chat/async_events_test.go が受け持つ。

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
	// notificationsChanged は「通知の状態が変わった」を知らせた回数（ユーザーごと）。
	// 数字は載らないイベントなので、数えるのは回数だけ。
	notificationsChanged map[int64]int
}

func newFakeSSEBroker() *fakeSSEBroker {
	return &fakeSSEBroker{notificationsChanged: map[int64]int{}}
}

func (f *fakeSSEBroker) PublishToUser(userID int64, eventType string, data map[string]any) {
	f.events = append(f.events, sseEvent{userID: userID, eventType: eventType, data: data})
}

func (f *fakeSSEBroker) PublishNotificationsChangedToUser(userID int64) {
	f.notificationsChanged[userID]++
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

func (f *fakeNotificationPublisher) PublishToAllActiveUsers(_ context.Context, _ notificationuc.BroadcastParams) error {
	return nil
}

// fakeCourseRegistrantIDs は授業ルームの room_changed の宛先（履修者）。
// 非授業ルームではここを通ってはいけない（宛先はイベントに載っている MemberIDs）ので、
// 呼ばれた回数を数えておく。
type fakeCourseRegistrantIDs struct {
	ids   []int64
	err   error
	calls int
}

func (f *fakeCourseRegistrantIDs) Execute(_ context.Context, _ int64) ([]int64, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.ids, nil
}

// inlineAsyncRunner は渡された配信をその場で実行する。本番の
// chatusecase.AsyncRunner と違って goroutine を使わないので、テストは
// 「非同期だから観測できない」を気にせず配信内容だけを見られる。
type inlineAsyncRunner struct{}

func (inlineAsyncRunner) Go(ctx context.Context, _ string, fn func(context.Context)) { fn(ctx) }

// pubsubOnlyRunner は非同期側の配信を一切実行せず、積まれた件数だけ数える。
// 「同期で配信されるもの／リクエストの外へ出されるもの」の境目を確かめるのに使う。
type pubsubOnlyRunner struct{ deferred int }

func (r *pubsubOnlyRunner) Go(context.Context, string, func(context.Context)) { r.deferred++ }

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

type publisherHarness struct {
	pubsub      *fakePubSub
	sse         *fakeSSEBroker
	notify      *fakeNotificationPublisher
	registrants *fakeCourseRegistrantIDs
	publisher   chatusecase.EventPublisher
}

func newPublisherHarness() *publisherHarness {
	return newPublisherHarnessWith(inlineAsyncRunner{})
}

func newPublisherHarnessWith(runner chatEventAsyncRunner) *publisherHarness {
	h := &publisherHarness{
		pubsub: newFakePubSub(),
		sse:    newFakeSSEBroker(),
		notify: &fakeNotificationPublisher{},
		// 送信者 10 を混ぜてあるのは、宛先から投稿者が外れることを確かめるため。
		registrants: &fakeCourseRegistrantIDs{ids: []int64{10, 21}},
	}
	h.publisher = NewChatEventPublisher(ChatEventPublisherDeps{
		PubSub:                h.pubsub,
		SSEBroker:             h.sse,
		NotificationPublisher: h.notify,
		CourseRegistrantIDs:   h.registrants,
		Async:                 runner,
		// 引用返信の通知先（返信元の投稿者）。返信を含むイベントを流したテストだけが使う。
		GetMessage:                     &fakeGetMessageByID{msg: &model.Message{ID: 99, UserID: 42}},
		GetAnonymousIdentity:           &fakeGetAnonymousIdentity{},
		MarkNotificationsAsReadByActor: &fakeMarkAllAsReadByActor{},
	})
	return h
}

// 授業ルームは room_users を使わないので、room_changed の宛先も専用の経路で引く。
// ここを取り違えると履修者に更新が1件も届かない。
func TestChatEventPublisher_MessageSent_CourseRoomUsesTheRegistrantRoute(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 1, Type: model.RoomTypeCourse}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:    room,
		Message: &model.Message{ID: 100, RoomID: 1, UserID: 10, Content: "hi"},
		ActorID: 10,
	})

	if h.registrants.calls != 1 {
		t.Fatalf("course registrant lookups = %d, want 1", h.registrants.calls)
	}

	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":message:added"]); got != 1 {
		t.Fatalf("message:added publishes = %d, want 1", got)
	}
	// 履修者は 10（＝投稿者）と 21 の2人。投稿者を除いた 21 にだけ届く。
	if len(h.sse.events) != 1 || h.sse.events[0].userID != 21 {
		t.Fatalf("room_changed events = %+v, want exactly one for the enrolled user 21", h.sse.events)
	}
	// 授業内チャットは匿名。DM 通知を出す相手も居ない。
	if len(h.notify.published) != 0 {
		t.Errorf("notifications = %+v, want none in a course room", h.notify.published)
	}
}

// room_changed のペイロードの形。
//
// unreadCount は載せない（未読数は受け取った側が自分ぶんだけ取り直す）。actorID も
// 載せない（授業内チャットは匿名なので、投稿者を特定できる情報を配ると匿名性が崩れる）。
// 代わりに宛先から投稿者を外してあるので hasNewMessage は常に true。
func TestChatEventPublisher_MessageSent_RoomChangedCarriesFactsOnly(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 1, Type: model.RoomTypeCourse}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:    room,
		Message: &model.Message{ID: 100, RoomID: 1, UserID: 10, Content: "hi"},
		ActorID: 10,
	})

	if len(h.sse.events) != 1 {
		t.Fatalf("room_changed events = %+v, want 1", h.sse.events)
	}
	ev := h.sse.events[0]
	if ev.eventType != "room_changed" {
		t.Errorf("event name = %q, want %q (unread_room は unreadCount ごと廃止した)", ev.eventType, "room_changed")
	}
	// sentAt は時刻なので固定値と比べられない。形だけ先に確かめて取り除き、
	// 残りを丸ごと突き合わせる（こうすると「増えたキー」も漏れなく捕まる）。
	sentAt := requireSentAt(t, ev.data)
	rest := map[string]any{}
	for k, v := range ev.data {
		if k != "sentAt" {
			rest[k] = v
		}
	}
	want := map[string]any{
		"roomID":        encodeGraphID("room", 1),
		"messageID":     encodeGraphID("message", 100),
		"hasNewMessage": true,
		"lastMessage":   "hi",
	}
	if !reflect.DeepEqual(rest, want) {
		t.Errorf("payload = %+v, want %+v plus sentAt", ev.data, want)
	}
	if delta := time.Since(time.Unix(0, sentAt)); delta < 0 || delta > time.Minute {
		t.Errorf("sentAt = %d (%v ago), want the time the event was built", sentAt, delta)
	}
	if _, ok := ev.data["unreadCount"]; ok {
		t.Error("payload carries unreadCount; the server must not compute unread counts for everyone")
	}
	if _, ok := ev.data["actorID"]; ok {
		t.Error("payload carries actorID; 授業内チャットの匿名性が崩れる")
	}
}

// 非授業ルームの宛先は room_users（権限判定で引いて既にイベントに載っている）。
// 授業用の経路は使わない（同じクエリを2度投げないための配線でもある）。
func TestChatEventPublisher_MessageSent_NonCourseRoomBroadcastsToMembersExceptTheSender(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 5, Type: model.RoomTypeCommunity}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:      room,
		Message:   &model.Message{ID: 100, RoomID: 5, UserID: 10, Content: "hi"},
		ActorID:   10,
		MemberIDs: []int64{10, 11, 12},
	})

	if h.registrants.calls != 0 {
		t.Fatalf("course registrant lookups = %d, want 0 outside course rooms", h.registrants.calls)
	}
	var got []int64
	for _, e := range h.sse.events {
		if e.eventType != "room_changed" {
			t.Fatalf("unexpected SSE event %q", e.eventType)
		}
		got = append(got, e.userID)
	}
	if !reflect.DeepEqual(got, []int64{11, 12}) {
		t.Errorf("room_changed recipients = %v, want the members except the sender (11, 12)", got)
	}
}

// DM は相手にだけ通知を出す（自分には出さない）。
func TestChatEventPublisher_MessageSent_DMNotifiesThePartnerOnly(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 3, Type: model.RoomTypeDM}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:      room,
		Message:   &model.Message{ID: 100, RoomID: 3, UserID: 10, Content: "hi"},
		ActorID:   10,
		MemberIDs: []int64{10, 11},
	})

	if h.registrants.calls != 0 {
		t.Fatalf("course registrant lookups = %d, want 0 in a DM", h.registrants.calls)
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

// 通知はベストエフォート。落ちても購読配信と room_changed は止めない
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
		t.Fatalf("room_changed events = %d, want 1 even when the notification fails", len(h.sse.events))
	}
}

// 宛先が引けないときは room_changed だけを諦める。購読配信と通知は通す。
func TestChatEventPublisher_MessageSent_RecipientLookupFailureSkipsOnlyTheBroadcast(t *testing.T) {
	h := newPublisherHarness()
	h.registrants.err = errors.New("db is down")
	room := &model.Room{ID: 1, Type: model.RoomTypeCourse}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:    room,
		Message: &model.Message{ID: 100, RoomID: 1, UserID: 10, Content: "hi", ReplyToID: ptrInt64(99)},
		ActorID: 10,
	})

	if len(h.sse.events) != 0 {
		t.Errorf("room_changed events = %+v, want none when the recipients cannot be resolved", h.sse.events)
	}
	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":message:added"]); got != 1 {
		t.Errorf("message:added publishes = %d, want 1", got)
	}
	if len(h.notify.published) != 1 {
		t.Errorf("reply notifications = %d, want 1 (宛先の取得と通知は別経路)", len(h.notify.published))
	}
}

// message:added はリクエストの中で同期に publish する。
//
// これを非同期にすると、同じルームの2件が入れ替わってチャット本体のメッセージが
// 逆順に表示されうる。安価なインメモリ publish なので、応答時間のために順序を
// 崩す取引に見合わない。逆に宛先ぶんの SSE と通知はリクエストの外へ出す。
func TestChatEventPublisher_MessageSent_PublishesTheMessageSynchronously(t *testing.T) {
	runner := &pubsubOnlyRunner{}
	h := newPublisherHarnessWith(runner)
	room := &model.Room{ID: 3, Type: model.RoomTypeDM}

	h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
		Room:      room,
		Message:   &model.Message{ID: 100, RoomID: 3, UserID: 10, Content: "hi"},
		ActorID:   10,
		MemberIDs: []int64{10, 11},
	})

	roomGraphID := encodeGraphID("room", room.ID)
	if got := len(h.pubsub.published[roomGraphID+":message:added"]); got != 1 {
		t.Errorf("message:added publishes = %d, want 1 without waiting for the async runner", got)
	}
	if runner.deferred != 1 {
		t.Errorf("deferred deliveries = %d, want 1 (SSE と通知はリクエストの外へ)", runner.deferred)
	}
	if len(h.sse.events) != 0 || len(h.notify.published) != 0 {
		t.Errorf("sse=%+v notifications=%+v, want both to be left to the async runner", h.sse.events, h.notify.published)
	}
}

// 削除も購読配信なので同期（削除が追加を追い越すと消えないメッセージが残る）。
func TestChatEventPublisher_MessageDeleted_PublishesSynchronously(t *testing.T) {
	runner := &pubsubOnlyRunner{}
	h := newPublisherHarnessWith(runner)

	h.publisher.MessageDeleted(context.Background(), chatusecase.MessageDeletedEvent{RoomID: 3, MessageID: 100})

	roomGraphID := encodeGraphID("room", 3)
	if got := len(h.pubsub.published[roomGraphID+":message:deleted"]); got != 1 {
		t.Errorf("message:deleted publishes = %d, want 1 without waiting for the async runner", got)
	}
	if runner.deferred != 0 {
		t.Errorf("deferred deliveries = %d, want 0 (削除は全て同期でよい)", runner.deferred)
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
	// 既読を打った本人（の別タブ・別端末）へ「更新された」ことだけを送る。
	// 数は送らない: 既読位置は画面に出した最後までなので 0 とは限らず、
	// 送れば嘘になりうる。受け取った側が自分のぶんを取り直せばよい。
	if len(h.sse.events) != 1 || h.sse.events[0].userID != 10 {
		t.Fatalf("room_changed events = %+v, want one for the reader", h.sse.events)
	}
	// sentAt も載せない。新旧の判定はプレビューの巻き戻しを防ぐためのもので、
	// プレビューを触らない既読の更新には要らない（chat_events.go の理由を参照）。
	want := map[string]any{"roomID": roomGraphID, "hasNewMessage": false}
	if !reflect.DeepEqual(h.sse.events[0].data, want) {
		t.Errorf("payload = %+v, want %+v", h.sse.events[0].data, want)
	}
}

// requireSentAt は sentAt が「10進文字列のナノ秒」であることを確かめて値を返す。
//
// 文字列であること自体がこのテストの主眼。数値で送ると受け手の JS が Number
// （安全な整数は 9.0e15 まで）へ丸め、1.7e18 の UnixNano は下位の桁＝同じミリ秒に
// 並んだ2件を分ける桁を失う。型が数値へ戻れば、順序判定は黙って粗くなる。
func requireSentAt(t *testing.T, payload map[string]any) int64 {
	t.Helper()

	raw, ok := payload["sentAt"]
	if !ok {
		t.Fatalf("payload = %+v, want a sentAt so the client can drop reordered events", payload)
	}
	str, ok := raw.(string)
	if !ok {
		t.Fatalf("sentAt = %#v (%T), want a decimal string (JSON の数値では JS 側で桁が落ちる)", raw, raw)
	}
	nanos, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		t.Fatalf("sentAt = %q, want nanoseconds as a decimal string: %v", str, err)
	}
	return nanos
}

// 同じルームへ続けて送ったとき、sentAt は後のイベントほど新しい。
// ここが逆転すると、受け取った側は新しい更新を「古い」と判断して捨ててしまう。
func TestChatEventPublisher_MessageSent_SentAtDoesNotGoBackwards(t *testing.T) {
	h := newPublisherHarness()
	room := &model.Room{ID: 5, Type: model.RoomTypeCommunity}

	for _, id := range []int64{100, 101} {
		h.publisher.MessageSent(context.Background(), chatusecase.MessageSentEvent{
			Room:      room,
			Message:   &model.Message{ID: id, RoomID: 5, UserID: 10, Content: "hi"},
			ActorID:   10,
			MemberIDs: []int64{10, 11},
		})
	}

	if len(h.sse.events) != 2 {
		t.Fatalf("room_changed events = %d, want 2", len(h.sse.events))
	}
	first := requireSentAt(t, h.sse.events[0].data)
	second := requireSentAt(t, h.sse.events[1].data)
	// 同値までは許す（時計の分解能が粗い環境では起こりうる）。受け取った側は同値を
	// 古いものとして捨てる約束なので、落ちるのはプレビューの更新1回だけで済む。
	if second < first {
		t.Errorf("sentAt: 1件目=%d 2件目=%d, want the later event to be >= the earlier one", first, second)
	}
}

// DM を既読にしたら、通知の既読化に続けて「通知の状態が変わった」を本人へ知らせること。
// 数字（未読通知数）は載らない: 受け取った側が自分で取り直す。
func TestChatEventPublisher_RoomMarkedAsRead_DMTellsTheReaderNotificationsChanged(t *testing.T) {
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
	if got := h.sse.notificationsChanged[10]; got != 1 {
		t.Errorf("notifications_changed for the reader = %d, want 1", got)
	}
	// 相手（11）は自分の通知を触られていないので知らせない。
	if got := h.sse.notificationsChanged[11]; got != 0 {
		t.Errorf("notifications_changed for the partner = %d, want 0", got)
	}
}

func ptrInt64(v int64) *int64 { return &v }
