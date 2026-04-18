package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RemoveUserFromRoomUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) error
}

var _ RemoveUserFromRoomUseCase = &RemoveUserFromRoomInteractor{}

type RemoveUserFromRoomInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewRemoveUserFromRoomUseCase(roomUserRepo repository.RoomUserRepository) RemoveUserFromRoomUseCase {
	return &RemoveUserFromRoomInteractor{roomUserRepo: roomUserRepo}
}

func (uc *RemoveUserFromRoomInteractor) Execute(ctx context.Context, roomID, userID int64) error {
	return uc.roomUserRepo.RemoveUserFromRoom(ctx, roomID, userID)
}
