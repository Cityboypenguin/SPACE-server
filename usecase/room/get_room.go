package room

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRoomUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Room, error)
}

var _ GetRoomUseCase = &GetRoomInteractor{}

type GetRoomInteractor struct {
	roomRepo repository.RoomRepository
}

func NewGetRoomUseCase(roomRepo repository.RoomRepository) GetRoomUseCase {
	return &GetRoomInteractor{roomRepo: roomRepo}
}

func (uc *GetRoomInteractor) Execute(ctx context.Context, id int64) (*model.Room, error) {
	r, err := uc.roomRepo.GetRoomByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("room not found: %d", id)
	}
	return r, nil
}
