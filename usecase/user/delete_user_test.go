package user

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type deleteUserRepo struct {
	repository.UserRepository
	deleted         bool
	activityDeleted bool
}

func (r *deleteUserRepo) DeleteUser(context.Context, int64) (bool, error) {
	r.deleted = true
	return true, nil
}

func (r *deleteUserRepo) DeleteActivityHistory(context.Context, int64) error {
	r.activityDeleted = true
	return nil
}

type deleteUserPostRepo struct{ repository.PostRepository }

func (*deleteUserPostRepo) DeletePostsByUserID(context.Context, int64) error { return nil }
func (*deleteUserPostRepo) RecalculateReplyCountsAffectedByUser(context.Context, int64) error {
	return nil
}

type deleteUserRoomRepo struct {
	repository.RoomRepository
	deletedRooms []int64
}

func (r *deleteUserRoomRepo) DeleteRoom(_ context.Context, roomID int64) (bool, error) {
	r.deletedRooms = append(r.deletedRooms, roomID)
	return true, nil
}

type deleteUserRoomUsers struct {
	repository.RoomUserRepository
	memberships map[int64]string
	roomRoles   map[int64]map[int64]string
}

func (r *deleteUserRoomUsers) LockUserCommunityMembershipsForUpdate(context.Context, int64) (map[int64]string, error) {
	return r.memberships, nil
}

func (r *deleteUserRoomUsers) LockRoomMemberRolesForUpdate(_ context.Context, roomID int64) (map[int64]string, error) {
	return r.roomRoles[roomID], nil
}

type inlineUserTx struct{}

func (inlineUserTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func deletingUserContext(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID, Role: "user"})
}

func TestDeleteUserRejectsSoleCommunityOwnerWithOtherMembers(t *testing.T) {
	users := &deleteUserRepo{}
	rooms := &deleteUserRoomRepo{}
	members := &deleteUserRoomUsers{
		memberships: map[int64]string{10: model.RoomUserRoleOwner},
		roomRoles: map[int64]map[int64]string{10: {
			1: model.RoomUserRoleOwner,
			2: model.RoomUserRoleMember,
		}},
	}
	uc := NewDeleteUserUseCase(users, &deleteUserPostRepo{}, rooms, members, inlineUserTx{})

	if _, err := uc.Execute(deletingUserContext(1), 1); err == nil {
		t.Fatal("sole owner deletion must be rejected")
	}
	if users.deleted || users.activityDeleted || len(rooms.deletedRooms) != 0 {
		t.Fatal("rejected account deletion changed persisted state")
	}
}

func TestDeleteUserDeletesCommunitiesWhereUserIsOnlyMember(t *testing.T) {
	users := &deleteUserRepo{}
	rooms := &deleteUserRoomRepo{}
	members := &deleteUserRoomUsers{
		memberships: map[int64]string{10: model.RoomUserRoleOwner},
		roomRoles:   map[int64]map[int64]string{10: {1: model.RoomUserRoleOwner}},
	}
	uc := NewDeleteUserUseCase(users, &deleteUserPostRepo{}, rooms, members, inlineUserTx{})

	ok, err := uc.Execute(deletingUserContext(1), 1)
	if err != nil || !ok {
		t.Fatalf("delete failed: ok=%v err=%v", ok, err)
	}
	if !users.deleted || !users.activityDeleted || len(rooms.deletedRooms) != 1 || rooms.deletedRooms[0] != 10 {
		t.Fatalf("userDeleted=%v activityDeleted=%v deletedRooms=%v", users.deleted, users.activityDeleted, rooms.deletedRooms)
	}
}
