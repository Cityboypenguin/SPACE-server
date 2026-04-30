package room

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateRoomUseCase interface {
	Execute(ctx context.Context, param model.CreateRoomParam) (*model.Room, error)
}

var _ CreateRoomUseCase = &CreateRoomInteractor{}

type CreateRoomInteractor struct {
	roomRepo repository.RoomRepository
}

func NewCreateRoomUseCase(roomRepo repository.RoomRepository) CreateRoomUseCase {
	return &CreateRoomInteractor{roomRepo: roomRepo}
}

func (uc *CreateRoomInteractor) Execute(ctx context.Context, param model.CreateRoomParam) (*model.Room, error) {
	now := time.Now()
	param.CreatedAt = now
	param.UpdatedAt = now
	r := &model.Room{}
	r.CreateRoom(param)
	if err := uc.roomRepo.SaveRoom(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}
