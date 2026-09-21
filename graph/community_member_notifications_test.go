package graph

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

type recordingCommunityMemberUpdater struct {
	communityID int64
	updates     []communityusecase.MemberUpdate
	roomID      int64
	err         error
}

func (u *recordingCommunityMemberUpdater) Execute(_ context.Context, communityID int64, updates []communityusecase.MemberUpdate) (int64, error) {
	u.communityID = communityID
	u.updates = append([]communityusecase.MemberUpdate(nil), updates...)
	return u.roomID, u.err
}

func TestDeprecatedCommunityMemberMutationsUseAtomicUpdaterAndNotify(t *testing.T) {
	tests := []struct {
		name             string
		action           communityusecase.MemberAction
		notificationType notificationuc.NotificationType
		call             func(*mutationResolver, context.Context, string, string) (bool, error)
	}{
		{"promote", communityusecase.MemberActionPromote, notificationuc.TypeCommunityRole, (*mutationResolver).PromoteToCommunityOwner},
		{"demote", communityusecase.MemberActionDemote, notificationuc.TypeCommunityRole, (*mutationResolver).DemoteFromCommunityOwner},
		{"kick", communityusecase.MemberActionKick, notificationuc.TypeCommunityKick, (*mutationResolver).KickUserFromCommunity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updater := &recordingCommunityMemberUpdater{roomID: 33}
			publisher := &fakeNotificationPublisher{}
			bus := &recordingBus{}
			resolver := &mutationResolver{&Resolver{
				CommunityUseCases:    CommunityUseCases{UpdateCommunityMembersUseCase: updater},
				NotificationUseCases: NotificationUseCases{NotificationPublisher: publisher},
				PubSub:               bus,
			}}
			ctx := auth.WithClaims(context.Background(), userClaims(7))

			ok, err := tt.call(resolver, ctx, encodeGraphID("community", 11), encodeGraphID("user", 22))
			if err != nil || !ok {
				t.Fatalf("mutation failed: ok=%v err=%v", ok, err)
			}
			if updater.communityID != 11 || len(updater.updates) != 1 || updater.updates[0].Action != tt.action {
				t.Fatalf("atomic updater got community=%d updates=%v", updater.communityID, updater.updates)
			}
			if len(publisher.batched) != 1 {
				t.Fatalf("notifications=%d, want 1", len(publisher.batched))
			}
			n := publisher.batched[0]
			if n.UserID != 22 || n.Type != tt.notificationType || n.TargetID == nil || *n.TargetID != 11 {
				t.Fatalf("unexpected notification: %+v", n)
			}
			if tt.action == communityusecase.MemberActionKick && (n.ActorID == nil || *n.ActorID != 7) {
				t.Fatalf("kick actor=%v, want 7", n.ActorID)
			}

			// 除名だけは「もうこの部屋を読めない」ので、購読中のタブへ
			// その場で伝える必要がある。昇格・降格は閲覧できるかを変えない
			// ので、合図を出すと無関係な購読者の判定を走らせるだけになる。
			wantSignals := 0
			if tt.action == communityusecase.MemberActionKick {
				wantSignals = 1
			}
			if len(bus.published) != wantSignals {
				t.Fatalf("access-changed signals = %d, want %d", len(bus.published), wantSignals)
			}
			if wantSignals == 1 {
				got := bus.published[0]
				if got.topic != roomAccessChangedTopic(33) {
					t.Fatalf("signal topic = %q, want %q", got.topic, roomAccessChangedTopic(33))
				}
				changed, ok := got.data.(*RoomAccessChanged)
				if !ok || len(changed.UserIDs) != 1 || changed.UserIDs[0] != 22 {
					t.Fatalf("signal payload = %+v, want the kicked user only", got.data)
				}
			}
		})
	}
}

func TestCommunityMemberNotificationFailureDoesNotRollbackCommittedUpdate(t *testing.T) {
	updater := &recordingCommunityMemberUpdater{}
	publisher := &fakeNotificationPublisher{err: context.Canceled}
	resolver := &Resolver{
		CommunityUseCases:    CommunityUseCases{UpdateCommunityMembersUseCase: updater},
		NotificationUseCases: NotificationUseCases{NotificationPublisher: publisher},
		PubSub:               &recordingBus{},
	}
	ctx := auth.WithClaims(context.Background(), userClaims(7))

	err := resolver.updateCommunityMembers(ctx, 11, []communityusecase.MemberUpdate{{
		UserID: 22,
		Action: communityusecase.MemberActionPromote,
	}})
	if err != nil {
		t.Fatalf("notification failure must remain best effort: %v", err)
	}
	if len(updater.updates) != 1 {
		t.Fatal("member update was not committed before notification")
	}
}

// recordingBus は Publish されたものだけを覚える pubsub.Bus。
// 購読は使わないので、受け取った側の挙動は subscription_stream_test.go が見る。
type recordingBus struct {
	published []publishedSignal
}

type publishedSignal struct {
	topic string
	data  interface{}
}

func (b *recordingBus) Subscribe(string) chan interface{}    { return make(chan interface{}) }
func (b *recordingBus) Unsubscribe(string, chan interface{}) {}
func (b *recordingBus) Publish(topic string, data interface{}) {
	b.published = append(b.published, publishedSignal{topic, data})
}
