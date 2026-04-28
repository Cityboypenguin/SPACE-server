package room

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type JoinRoomUseCase interface {
	Execute(ctx context.Context, roomID int64) (bool, error)
}

var _ JoinRoomUseCase = &JoinRoomInteractor{}

type JoinRoomInteractor struct {
	roomRepo     repository.RoomRepository
	roomUserRepo repository.RoomUserRepository
}

func NewJoinRoomUseCase(roomRepo repository.RoomRepository, roomUserRepo repository.RoomUserRepository) JoinRoomUseCase {
	return &JoinRoomInteractor{roomRepo: roomRepo, roomUserRepo: roomUserRepo}
}

func (uc *JoinRoomInteractor) Execute(ctx context.Context, roomID int64) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}

	room, err := uc.roomRepo.GetRoomByID(ctx, roomID)
	if err != nil {
		return false, err
	}
	if room == nil {
		return false, errors.New("room not found")
	}
	if room.Type != model.RoomTypeCommunity {
		return false, errors.New("can only join community rooms")
	}

	if err := uc.roomUserRepo.AddUserToRoom(ctx, roomID, claims.ID); err != nil {
		return false, err
	}
	return true, nil
}
