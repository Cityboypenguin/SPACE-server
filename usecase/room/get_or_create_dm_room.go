package room

import (
	"context"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetOrCreateDMRoomUseCase interface {
	Execute(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
}

var _ GetOrCreateDMRoomUseCase = &GetOrCreateDMRoomInteractor{}

type GetOrCreateDMRoomInteractor struct {
	roomRepo     repository.RoomRepository
	roomUserRepo repository.RoomUserRepository
}

func NewGetOrCreateDMRoomUseCase(roomRepo repository.RoomRepository, roomUserRepo repository.RoomUserRepository) GetOrCreateDMRoomUseCase {
	return &GetOrCreateDMRoomInteractor{
		roomRepo:     roomRepo,
		roomUserRepo: roomUserRepo,
	}
}

func (uc *GetOrCreateDMRoomInteractor) Execute(ctx context.Context, userID1, userID2 int64) (*model.Room, error) {
	existing, err := uc.roomUserRepo.FindDMRoom(ctx, userID1, userID2)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	now := time.Now().Unix()
	r := &model.Room{
		Name:        fmt.Sprintf("dm-%d-%d", userID1, userID2),
		Type:        "dm",
		Description: "Direct Message",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := uc.roomRepo.SaveRoom(ctx, r); err != nil {
		return nil, err
	}
	if err := uc.roomUserRepo.AddUserToRoom(ctx, r.ID, userID1); err != nil {
		return nil, err
	}
	if err := uc.roomUserRepo.AddUserToRoom(ctx, r.ID, userID2); err != nil {
		return nil, err
	}
	return r, nil
}
