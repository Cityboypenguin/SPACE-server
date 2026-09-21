package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListRoomMembersWithRolesPageUseCase interface {
	Execute(ctx context.Context, roomID int64, q repository.PageQuery) ([]*model.RoomMember, int, error)
}

type ListRoomMembersWithRolesPageInteractor struct {
	roomUserRepo repository.RoomMembershipReader
}

func NewListRoomMembersWithRolesPageUseCase(roomUserRepo repository.RoomMembershipReader) ListRoomMembersWithRolesPageUseCase {
	return &ListRoomMembersWithRolesPageInteractor{roomUserRepo: roomUserRepo}
}

func (uc *ListRoomMembersWithRolesPageInteractor) Execute(ctx context.Context, roomID int64, q repository.PageQuery) ([]*model.RoomMember, int, error) {
	return uc.roomUserRepo.ListRoomMembersWithRolesPage(ctx, roomID, q)
}
