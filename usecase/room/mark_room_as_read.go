package room

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MarkRoomAsReadUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) error
}

type markRoomAsReadUseCase struct {
	roomUserRepo repository.RoomUserRepository
}

func NewMarkRoomAsReadUseCase(roomUserRepo repository.RoomUserRepository) MarkRoomAsReadUseCase {
	return &markRoomAsReadUseCase{roomUserRepo: roomUserRepo}
}

func (uc *markRoomAsReadUseCase) Execute(ctx context.Context, roomID, userID int64) error {
	return uc.roomUserRepo.UpdateLastReadAt(ctx, roomID, userID, time.Now().Unix())
}
