package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AddUserToRoomUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) error
}

var _ AddUserToRoomUseCase = &AddUserToRoomInteractor{}

type AddUserToRoomInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewAddUserToRoomUseCase(roomUserRepo repository.RoomUserRepository) AddUserToRoomUseCase {
	return &AddUserToRoomInteractor{roomUserRepo: roomUserRepo}
}

func (uc *AddUserToRoomInteractor) Execute(ctx context.Context, roomID, userID int64) error {
	return uc.roomUserRepo.AddUserToRoom(ctx, roomID, userID)
}
