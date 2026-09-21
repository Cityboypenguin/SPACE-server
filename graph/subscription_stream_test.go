package graph

import (
	"context"
	"errors"
	"runtime"
	"sync"
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

	messages := subscribeTopicFunc(ctx, ps, "room-1:message:added", scope, identity[*gqlmodel.Message])
	questions := subscribeTopicFunc(ctx, ps, "room-1:question:added", scope, identity[*gqlmodel.Question])
	answers := subscribeTopicFunc(ctx, ps, "question-1:answer:added", scope, identity[*gqlmodel.Answer])
	polls := subscribeTopicFunc(ctx, ps, "room-1:poll:added", scope, identity[*gqlmodel.Poll])

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

	ch := subscribeTopicFunc(ctx, ps, "room-1:message:added", subscriptionScope{UserID: 1}, identity[*gqlmodel.Message])

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

	ch := subscribeTopicFunc(ctx, ps, "room-1:message:added", subscriptionScope{UserID: 1}, identity[*gqlmodel.Message])
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

// countingSource は Unsubscribe が呼ばれたかを観測するための購読元。
// *pubsub.PubSub でも「購読が外れたか」は間接的にしか見えないので、
// 転送ループの後始末だけを直接確かめたいときはこちらを使う。
type countingSource struct {
	ch          chan interface{}
	unsubscribe chan struct{} // Unsubscribe で1度だけ閉じる
	once        sync.Once
}

func newCountingSource() *countingSource {
	return &countingSource{
		ch:          make(chan interface{}, 64),
		unsubscribe: make(chan struct{}),
	}
}

func (s *countingSource) Subscribe(string) chan interface{} { return s.ch }

func (s *countingSource) Unsubscribe(string, chan interface{}) {
	s.once.Do(func() { close(s.unsubscribe) })
}

// 読み手が居ないまま送信で詰まっていても、ctx のキャンセルで転送 goroutine が
// 終わり、購読が外れ、出力チャンネルが閉じること。
//
// out はバッファ1なので、2件流すと2件目の送信でブロックする。以前はその送信が
// キャンセル不可で、この状況（クライアントが読まずに切断）だと goroutine と購読が
// 永久に残っていた。
func TestSubscribeTopic_UnsubscribesWhenSendBlocksAndContextIsCanceled(t *testing.T) {
	src := newCountingSource()
	ctx, cancel := context.WithCancel(context.Background())

	out := subscribeTopicFunc(ctx, src, "room-1:message:added", subscriptionScope{UserID: 1}, identity[*gqlmodel.Message])

	// 1件目は out のバッファへ入り、2件目の送信でループが止まる。
	src.ch <- &gqlmodel.Message{ID: "m1"}
	src.ch <- &gqlmodel.Message{ID: "m2"}

	// 送信待ちに入るまで待つ（バッファが埋まる＝1件目が out に入っている）。
	deadline := time.After(2 * time.Second)
	for len(out) == 0 {
		select {
		case <-deadline:
			t.Fatal("forwarding loop never filled the output buffer")
		default:
		}
		runtime.Gosched()
	}

	cancel()

	select {
	case <-src.unsubscribe:
	case <-time.After(2 * time.Second):
		t.Fatal("Unsubscribe was not called after cancellation while the send was blocked")
	}

	// バッファに残った1件を読み切ると、閉じられていることも確かめられる。
	<-out
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("expected the subscription channel to be closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription channel was not closed after cancellation")
	}
}

// --- 購読中の権限の確かめ直し（項目2） ------------------------------------

// shortenAccessRecheck はテストのあいだだけ確かめ直しの間隔を縮める。
func shortenAccessRecheck(t *testing.T, d time.Duration) {
	t.Helper()
	prev := accessRecheckInterval
	accessRecheckInterval = d
	t.Cleanup(func() { accessRecheckInterval = prev })
}

// TestSubscribeTopicGuarded_StopsDeliveringOnceAccessIsRevoked は項目2の本体。
//
// 権限は購読を始めるときにしか見ていなかったので、退出・キックされた利用者の
// 繋ぎっぱなしのタブに、その部屋の新着が流れ続けていた。普通に使っている限り
// 画面には出ないため、見て気づける壊れ方ではない。
func TestSubscribeTopicGuarded_StopsDeliveringOnceAccessIsRevoked(t *testing.T) {
	shortenAccessRecheck(t, time.Millisecond)

	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	allowed := true
	authorize := func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		if allowed {
			return nil
		}
		return errors.New("forbidden: not a member of this room")
	}

	out := subscribeTopicGuarded(ctx, ps, "room-1:message:added",
		subscriptionScope{UserID: 7, RoomID: "room-1"},
		subscriptionGuard{authorize: authorize}, identity[*gqlmodel.Message])

	// 在籍しているうちは届く。
	time.Sleep(2 * time.Millisecond)
	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "before"})
	if got := recvWithin(t, out); got.ID != "before" {
		t.Fatalf("id = %q, want before", got.ID)
	}

	// ここでキックされたとする。
	mu.Lock()
	allowed = false
	mu.Unlock()

	time.Sleep(2 * time.Millisecond)
	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "after"})

	// 権限を失ったあとの新着は届かず、購読そのものが終わる
	// （チャンネルが閉じることで gqlgen 側の subscription も完了する）。
	select {
	case got, ok := <-out:
		if ok {
			t.Fatalf("権限を失ったあとに %+v が配信された", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("購読が終わらない（チャンネルが閉じていない）")
	}
}

// TestSubscribeTopicGuarded_KeepsDeliveringWhileAuthorized は、確かめ直しが
// 正当な購読まで切ってしまわないことを確かめる。
func TestSubscribeTopicGuarded_KeepsDeliveringWhileAuthorized(t *testing.T) {
	shortenAccessRecheck(t, time.Millisecond)

	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := subscribeTopicGuarded(ctx, ps, "room-1:message:added",
		subscriptionScope{UserID: 7, RoomID: "room-1"},
		subscriptionGuard{authorize: func(context.Context) error { return nil }},
		identity[*gqlmodel.Message])

	for _, id := range []string{"m1", "m2", "m3"} {
		time.Sleep(2 * time.Millisecond)
		ps.Publish("room-1:message:added", &gqlmodel.Message{ID: id})
		if got := recvWithin(t, out); got.ID != id {
			t.Fatalf("id = %q, want %q", got.ID, id)
		}
	}
}

// TestSubscribeTopicGuarded_ThrottlesTheRecheck は、確かめ直しが配信のたびには
// 走らないことを確かめる。賑やかな部屋では「メッセージ数 × 購読者数」だけ
// 判定が走ることになり、これは権限の正しさではなく費用の問題として効いてくる。
func TestSubscribeTopicGuarded_ThrottlesTheRecheck(t *testing.T) {
	// 間隔を長くして、このテストの間は1度も確かめ直しが起きないようにする。
	shortenAccessRecheck(t, time.Hour)

	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	calls := 0
	out := subscribeTopicGuarded(ctx, ps, "room-1:message:added",
		subscriptionScope{UserID: 7, RoomID: "room-1"},
		subscriptionGuard{authorize: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return nil
		}},
		identity[*gqlmodel.Message])

	for i := range 5 {
		ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "m"})
		recvWithin(t, out)
		_ = i
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatalf("recheck calls = %d, want 0（開始時に判定したばかりなので間隔内は引き直さない）", calls)
	}
}

// TestSubscribeTopicGuarded_SkipsTheRecheckForFilteredEvents は、絞り込みで
// 落とす値のために判定を走らせないことを確かめる。既読の購読は自分のイベントを
// 落とすので、自分が読むたびに判定が走ると無駄が大きい。
func TestSubscribeTopicGuarded_SkipsTheRecheckForFilteredEvents(t *testing.T) {
	shortenAccessRecheck(t, time.Millisecond)

	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	calls := 0
	out := subscribeTopicGuarded(ctx, ps, "room-1:read_status",
		subscriptionScope{UserID: 7, RoomID: "room-1"},
		subscriptionGuard{authorize: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return nil
		}},
		func(m *gqlmodel.Message) (*gqlmodel.Message, bool) {
			// 全部落とす（自分のイベントだけが流れてきた状況に相当）。
			return m, false
		})

	for range 5 {
		time.Sleep(2 * time.Millisecond)
		ps.Publish("room-1:read_status", &gqlmodel.Message{ID: "mine"})
	}
	expectNothing(t, out)

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatalf("recheck calls = %d, want 0（配信しない値のために判定しない）", calls)
	}
}

// --- 権限が変わったときの即時の確かめ直し ---------------------------------

// TestSubscribeTopicGuarded_RechecksImmediatelyOnRevokeSignal は、合図が来たら
// accessRecheckInterval を待たずに購読が終わることを確かめる。
//
// 受け皿（間隔ごとの確かめ直し）だけだと、キックされてから最大30秒のあいだ
// その部屋の新着が開いたままのタブへ届き続ける。合図はその窓を潰すためにある。
func TestSubscribeTopicGuarded_RechecksImmediatelyOnRevokeSignal(t *testing.T) {
	// 受け皿は効かせない。これで「終わったのは合図のおかげ」だと言い切れる。
	shortenAccessRecheck(t, time.Hour)

	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	allowed := true
	guard := subscriptionGuard{
		authorize: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			if allowed {
				return nil
			}
			return errors.New("forbidden: not a member of this room")
		},
		revokeTopic: "room-1:access:changed",
	}

	out := subscribeTopicGuarded(ctx, ps, "room-1:message:added",
		subscriptionScope{UserID: 7, RoomID: "room-1"}, guard, identity[*gqlmodel.Message])

	time.Sleep(2 * time.Millisecond)
	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "before"})
	if got := recvWithin(t, out); got.ID != "before" {
		t.Fatalf("id = %q, want before", got.ID)
	}

	// キックされ、その場で合図が出る。
	mu.Lock()
	allowed = false
	mu.Unlock()
	ps.Publish("room-1:access:changed", &RoomAccessChanged{UserIDs: []int64{7}})

	select {
	case got, ok := <-out:
		if ok {
			t.Fatalf("合図のあとに %+v が配信された", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("合図を受けても購読が終わらない（間隔を待ってしまっている）")
	}
}

// TestSubscribeTopicGuarded_IgnoresRevokeSignalsForOtherUsers は、他人あての
// 合図で自分の購読まで確かめ直さないことを確かめる。
//
// 20人まとめてキックするようなときに全員ぶんの合図が飛ぶので、宛先で絞らないと
// 無関係な購読者の数だけ判定が走る（賑やかな部屋ほど効いてくる）。
func TestSubscribeTopicGuarded_IgnoresRevokeSignalsForOtherUsers(t *testing.T) {
	shortenAccessRecheck(t, time.Hour)

	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	calls := 0
	guard := subscriptionGuard{
		authorize: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return nil
		},
		revokeTopic: "room-1:access:changed",
	}

	out := subscribeTopicGuarded(ctx, ps, "room-1:message:added",
		subscriptionScope{UserID: 7, RoomID: "room-1"}, guard, identity[*gqlmodel.Message])

	time.Sleep(2 * time.Millisecond)
	ps.Publish("room-1:access:changed", &RoomAccessChanged{UserIDs: []int64{8, 9}})

	// 合図を処理しきったことを、後続の配信が届くことで確かめる。
	time.Sleep(2 * time.Millisecond)
	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "m1"})
	if got := recvWithin(t, out); got.ID != "m1" {
		t.Fatalf("id = %q, want m1", got.ID)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatalf("recheck calls = %d, want 0（他人あての合図では引き直さない）", calls)
	}
}

// TestSubscribeTopicGuarded_KeepsSubscriptionWhenTheSignalWasNotARevocation は、
// 合図が来ても権限が残っていれば購読が続くことを確かめる。
//
// 合図は「変わったかもしれない」までしか言わない。切るかどうかを決めるのは
// 判定1本（requireRoomReadAccess）で、合図そのものではない。
func TestSubscribeTopicGuarded_KeepsSubscriptionWhenTheSignalWasNotARevocation(t *testing.T) {
	shortenAccessRecheck(t, time.Hour)

	ps := pubsub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	guard := subscriptionGuard{
		authorize:   func(context.Context) error { return nil },
		revokeTopic: "room-1:access:changed",
	}

	out := subscribeTopicGuarded(ctx, ps, "room-1:message:added",
		subscriptionScope{UserID: 7, RoomID: "room-1"}, guard, identity[*gqlmodel.Message])

	time.Sleep(2 * time.Millisecond)
	ps.Publish("room-1:access:changed", &RoomAccessChanged{UserIDs: []int64{7}})

	time.Sleep(2 * time.Millisecond)
	ps.Publish("room-1:message:added", &gqlmodel.Message{ID: "m1"})
	if got := recvWithin(t, out); got.ID != "m1" {
		t.Fatalf("id = %q, want m1（権限が残っていれば合図では切らない）", got.ID)
	}
}

// topicSource はトピックごとに別のチャンネルを返す購読元。
//
// *pubsub.PubSub と違い、購読が始まる**前**に値を積んでおける。合図と新着が
// 両方とも待っている状態をテストから作るのに要る（本物では、goroutine が
// 動き出す前に両方を揃えることができない）。
type topicSource struct {
	mu  sync.Mutex
	chs map[string]chan interface{}
}

func newTopicSource() *topicSource {
	return &topicSource{chs: make(map[string]chan interface{})}
}

func (s *topicSource) channel(topic string) chan interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chs[topic] == nil {
		s.chs[topic] = make(chan interface{}, 8)
	}
	return s.chs[topic]
}

func (s *topicSource) Subscribe(topic string) chan interface{} { return s.channel(topic) }

func (s *topicSource) Unsubscribe(string, chan interface{}) {}

func (s *topicSource) push(topic string, v interface{}) { s.channel(topic) <- v }

// TestSubscribeTopicGuarded_DoesNotDeliverWhenASignalIsAlreadyWaiting は、
// 合図と新着が同時に待っているときでも配信しないことを確かめる。
//
// select は準備できている case が複数あると無作為に1つ選ぶ。合図を別の case で
// 待つだけでは、新着の方が先に選ばれたときに素通りしてしまう（間隔内なら
// 確かめ直しも走らない）。配信の直前に届いている合図を片付けることで、
// 「その時点で合図が届いていたなら必ず先に効く」まで詰められる。
//
// 無作為なので、1回では当たり外れが出る。繰り返して「1度も漏れない」ことを見る。
func TestSubscribeTopicGuarded_DoesNotDeliverWhenASignalIsAlreadyWaiting(t *testing.T) {
	// 受け皿は効かせない。効いてしまうと、合図の有無に関係なく止まる。
	shortenAccessRecheck(t, time.Hour)

	for i := range 50 {
		func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			src := newTopicSource()
			// 購読が始まる前に両方を積む。ループが動き出した時点で
			// 合図も新着も待っている状態になる。
			src.push("room-1:access:changed", &RoomAccessChanged{UserIDs: []int64{7}})
			src.push("room-1:message:added", &gqlmodel.Message{ID: "after-kick"})

			guard := subscriptionGuard{
				authorize:   func(context.Context) error { return errors.New("forbidden: not a member of this room") },
				revokeTopic: "room-1:access:changed",
			}
			out := subscribeTopicGuarded(ctx, src, "room-1:message:added",
				subscriptionScope{UserID: 7, RoomID: "room-1"}, guard, identity[*gqlmodel.Message])

			select {
			case got, ok := <-out:
				if ok {
					t.Fatalf("%d 回目: キック済みなのに %+v が配信された", i, got)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%d 回目: 購読が終わらない", i)
			}
		}()
	}
}
