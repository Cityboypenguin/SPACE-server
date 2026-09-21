package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SetRoomUserRoleUseCase interface {
	Execute(ctx context.Context, roomID, userID int64, role string) error
}

var _ SetRoomUserRoleUseCase = &SetRoomUserRoleInteractor{}

type SetRoomUserRoleInteractor struct {
	roomUserRepo repository.RoomRoleRepository
}

func NewSetRoomUserRoleUseCase(roomUserRepo repository.RoomRoleRepository) SetRoomUserRoleUseCase {
	return &SetRoomUserRoleInteractor{roomUserRepo: roomUserRepo}
}

func (uc *SetRoomUserRoleInteractor) Execute(ctx context.Context, roomID, userID int64, role string) error {
	return uc.roomUserRepo.SetRoomUserRole(ctx, roomID, userID, role)
}
