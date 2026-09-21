package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetUserIDsByRoomIDUseCase interface {
	Execute(ctx context.Context, roomID int64) ([]int64, error)
}

var _ GetUserIDsByRoomIDUseCase = &GetUserIDsByRoomIDInteractor{}

type GetUserIDsByRoomIDInteractor struct {
	roomUserRepo repository.RoomMembershipReader
}

func NewGetUserIDsByRoomIDUseCase(roomUserRepo repository.RoomMembershipReader) GetUserIDsByRoomIDUseCase {
	return &GetUserIDsByRoomIDInteractor{roomUserRepo: roomUserRepo}
}

func (uc *GetUserIDsByRoomIDInteractor) Execute(ctx context.Context, roomID int64) ([]int64, error) {
	return uc.roomUserRepo.GetUserIDsByRoomID(ctx, roomID)
}
