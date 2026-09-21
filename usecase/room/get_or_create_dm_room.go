package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetOrCreateDMRoomUseCase interface {
	Execute(ctx context.Context, userID2 int64) (*model.Room, error)
}

var _ GetOrCreateDMRoomUseCase = &GetOrCreateDMRoomInteractor{}

type GetOrCreateDMRoomInteractor struct {
	roomUserRepo repository.DMRoomRepository
}

func NewGetOrCreateDMRoomUseCase(roomUserRepo repository.DMRoomRepository) GetOrCreateDMRoomUseCase {
	return &GetOrCreateDMRoomInteractor{
		roomUserRepo: roomUserRepo,
	}
}

func (uc *GetOrCreateDMRoomInteractor) Execute(ctx context.Context, userID2 int64) (*model.Room, error) {
	userID1, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	return uc.roomUserRepo.FindOrCreateDMRoom(ctx, userID1, userID2)
}
