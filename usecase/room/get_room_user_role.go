package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRoomUserRoleUseCase interface {
	Execute(ctx context.Context, roomID int64) (string, error)
}

var _ GetRoomUserRoleUseCase = &GetRoomUserRoleInteractor{}

type GetRoomUserRoleInteractor struct {
	roomUserRepo repository.RoomRoleRepository
}

func NewGetRoomUserRoleUseCase(roomUserRepo repository.RoomRoleRepository) GetRoomUserRoleUseCase {
	return &GetRoomUserRoleInteractor{roomUserRepo: roomUserRepo}
}

func (uc *GetRoomUserRoleInteractor) Execute(ctx context.Context, roomID int64) (string, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return "", err
	}
	return uc.roomUserRepo.GetRoomUserRole(ctx, roomID, userID)
}
