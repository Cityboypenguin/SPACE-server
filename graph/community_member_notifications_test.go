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
	err         error
}

func (u *recordingCommunityMemberUpdater) Execute(_ context.Context, communityID int64, updates []communityusecase.MemberUpdate) error {
	u.communityID = communityID
	u.updates = append([]communityusecase.MemberUpdate(nil), updates...)
	return u.err
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
			updater := &recordingCommunityMemberUpdater{}
			publisher := &fakeNotificationPublisher{}
			resolver := &mutationResolver{&Resolver{
				CommunityUseCases:    CommunityUseCases{UpdateCommunityMembersUseCase: updater},
				NotificationUseCases: NotificationUseCases{NotificationPublisher: publisher},
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
		})
	}
}

func TestCommunityMemberNotificationFailureDoesNotRollbackCommittedUpdate(t *testing.T) {
	updater := &recordingCommunityMemberUpdater{}
	publisher := &fakeNotificationPublisher{err: context.Canceled}
	resolver := &Resolver{
		CommunityUseCases:    CommunityUseCases{UpdateCommunityMembersUseCase: updater},
		NotificationUseCases: NotificationUseCases{NotificationPublisher: publisher},
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
