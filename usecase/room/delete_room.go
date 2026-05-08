package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteRoomUseCase interface {
	Execute(ctx context.Context, roomID int64) (bool, error)
}

var _ DeleteRoomUseCase = &DeleteRoomInteractor{}

type DeleteRoomInteractor struct {
	roomRepo repository.RoomRepository
}

func NewDeleteRoomUseCase(roomRepo repository.RoomRepository) DeleteRoomUseCase {
	return &DeleteRoomInteractor{roomRepo: roomRepo}
}

func (uc *DeleteRoomInteractor) Execute(ctx context.Context, roomID int64) (bool, error) {
	return uc.roomRepo.DeleteRoom(ctx, roomID)
}
