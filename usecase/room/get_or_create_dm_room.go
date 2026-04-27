package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetOrCreateDMRoomUseCase interface {
	Execute(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
}

var _ GetOrCreateDMRoomUseCase = &GetOrCreateDMRoomInteractor{}

type GetOrCreateDMRoomInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewGetOrCreateDMRoomUseCase(roomUserRepo repository.RoomUserRepository) GetOrCreateDMRoomUseCase {
	return &GetOrCreateDMRoomInteractor{
		roomUserRepo: roomUserRepo,
	}
}

func (uc *GetOrCreateDMRoomInteractor) Execute(ctx context.Context, userID1, userID2 int64) (*model.Room, error) {
	return uc.roomUserRepo.FindOrCreateDMRoom(ctx, userID1, userID2)
}
