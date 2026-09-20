package room

import (
	"context"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type membershipRoomRepo struct {
	repository.RoomRepository
	room        *model.Room
	deleteCalls int
}

func (r *membershipRoomRepo) GetRoomByID(context.Context, int64) (*model.Room, error) {
	return r.room, nil
}
func (r *membershipRoomRepo) DeleteRoom(context.Context, int64) (bool, error) {
	r.deleteCalls++
	return true, nil
}

type membershipRoomUserRepo struct {
	repository.RoomUserRepository
	roles map[int64]string
}

func (r *membershipRoomUserRepo) LockRoomMemberRolesForUpdate(context.Context, int64) (map[int64]string, error) {
	result := make(map[int64]string, len(r.roles))
	for id, role := range r.roles {
		result[id] = role
	}
	return result, nil
}

func (r *membershipRoomUserRepo) RemoveUserFromRoom(_ context.Context, _ int64, userID int64) error {
	delete(r.roles, userID)
	return nil
}

type serialRoomTx struct{ mu sync.Mutex }

func (t *serialRoomTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fn(ctx)
}

func roomUserContext(id int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: id, Role: "user"})
}

func TestLeaveCommunityRejectsLastOwnerWithOtherMembers(t *testing.T) {
	rooms := &membershipRoomRepo{room: &model.Room{ID: 1, Type: model.RoomTypeCommunity}}
	members := &membershipRoomUserRepo{roles: map[int64]string{1: model.RoomUserRoleOwner, 2: model.RoomUserRoleMember}}
	uc := NewLeaveCommunityUseCase(rooms, members, &serialRoomTx{})

	if _, err := uc.Execute(roomUserContext(1), 1); err == nil {
		t.Fatal("last owner must not leave while another member remains")
	}
	if len(members.roles) != 2 || rooms.deleteCalls != 0 {
		t.Fatal("rejected leave changed persisted state")
	}
}

func TestLeaveCommunityDeletesRoomForLastMember(t *testing.T) {
	rooms := &membershipRoomRepo{room: &model.Room{ID: 1, Type: model.RoomTypeCommunity}}
	members := &membershipRoomUserRepo{roles: map[int64]string{1: model.RoomUserRoleOwner}}
	uc := NewLeaveCommunityUseCase(rooms, members, &serialRoomTx{})

	ok, err := uc.Execute(roomUserContext(1), 1)
	if err != nil || !ok || rooms.deleteCalls != 1 {
		t.Fatalf("last-member leave: ok=%v deletes=%d err=%v", ok, rooms.deleteCalls, err)
	}
}

func TestConcurrentOwnerLeavesKeepAnOwnerWhenMembersRemain(t *testing.T) {
	rooms := &membershipRoomRepo{room: &model.Room{ID: 1, Type: model.RoomTypeCommunity}}
	members := &membershipRoomUserRepo{roles: map[int64]string{
		1: model.RoomUserRoleOwner,
		2: model.RoomUserRoleOwner,
		3: model.RoomUserRoleMember,
	}}
	tx := &serialRoomTx{}
	uc := NewLeaveCommunityUseCase(rooms, members, tx)

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, userID := range []int64{1, 2} {
		go func(id int64) {
			<-start
			_, err := uc.Execute(roomUserContext(id), 1)
			results <- err
		}(userID)
	}
	close(start)

	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful leaves=%d, want 1", successes)
	}
	owners := 0
	for _, role := range members.roles {
		if role == model.RoomUserRoleOwner {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("owners=%d, want 1", owners)
	}
}

func TestDeleteOrphanedDMRequiresSingleRemainingCaller(t *testing.T) {
	rooms := &membershipRoomRepo{room: &model.Room{ID: 1, Type: model.RoomTypeDM}}
	members := &membershipRoomUserRepo{roles: map[int64]string{1: model.RoomUserRoleMember, 2: model.RoomUserRoleMember}}
	uc := NewDeleteOrphanedDMUseCase(rooms, members, &serialRoomTx{})

	if _, err := uc.Execute(roomUserContext(1), 1); err == nil {
		t.Fatal("active DM must not be deleted")
	}
	delete(members.roles, 2)
	ok, err := uc.Execute(roomUserContext(1), 1)
	if err != nil || !ok || rooms.deleteCalls != 1 {
		t.Fatalf("orphaned DM delete: ok=%v deletes=%d err=%v", ok, rooms.deleteCalls, err)
	}
}
