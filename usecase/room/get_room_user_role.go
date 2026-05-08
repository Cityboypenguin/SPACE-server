package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRoomUserRoleUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) (string, error)
}

var _ GetRoomUserRoleUseCase = &GetRoomUserRoleInteractor{}

type GetRoomUserRoleInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewGetRoomUserRoleUseCase(roomUserRepo repository.RoomUserRepository) GetRoomUserRoleUseCase {
	return &GetRoomUserRoleInteractor{roomUserRepo: roomUserRepo}
}

func (uc *GetRoomUserRoleInteractor) Execute(ctx context.Context, roomID, userID int64) (string, error) {
	return uc.roomUserRepo.GetRoomUserRole(ctx, roomID, userID)
}
