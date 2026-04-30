package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListUsersByRoomIDsUseCase interface {
	Execute(ctx context.Context, roomIDs []int64) (map[int64][]*model.User, error)
}

var _ ListUsersByRoomIDsUseCase = &ListUsersByRoomIDsInteractor{}

type ListUsersByRoomIDsInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewListUsersByRoomIDsUseCase(roomUserRepo repository.RoomUserRepository) ListUsersByRoomIDsUseCase {
	return &ListUsersByRoomIDsInteractor{roomUserRepo: roomUserRepo}
}

func (uc *ListUsersByRoomIDsInteractor) Execute(ctx context.Context, roomIDs []int64) (map[int64][]*model.User, error) {
	return uc.roomUserRepo.ListUsersByRoomIDs(ctx, roomIDs)
}
