package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListRoomMembersWithRolesUseCase interface {
	Execute(ctx context.Context, roomID int64) ([]*model.RoomMember, error)
}

var _ ListRoomMembersWithRolesUseCase = &ListRoomMembersWithRolesInteractor{}

type ListRoomMembersWithRolesInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewListRoomMembersWithRolesUseCase(roomUserRepo repository.RoomUserRepository) ListRoomMembersWithRolesUseCase {
	return &ListRoomMembersWithRolesInteractor{roomUserRepo: roomUserRepo}
}

func (uc *ListRoomMembersWithRolesInteractor) Execute(ctx context.Context, roomID int64) ([]*model.RoomMember, error) {
	return uc.roomUserRepo.ListRoomMembersWithRoles(ctx, roomID)
}
