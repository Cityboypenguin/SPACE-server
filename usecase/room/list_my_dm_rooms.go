package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListMyDMRoomsUseCase interface {
	Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Room, int, error)
}

var _ ListMyDMRoomsUseCase = &ListMyDMRoomsInteractor{}

type ListMyDMRoomsInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewListMyDMRoomsUseCase(roomUserRepo repository.RoomUserRepository) ListMyDMRoomsUseCase {
	return &ListMyDMRoomsInteractor{roomUserRepo: roomUserRepo}
}

func (uc *ListMyDMRoomsInteractor) Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Room, int, error) {
	return uc.roomUserRepo.ListDMRoomsByUserID(ctx, userID, limit, offset)
}
