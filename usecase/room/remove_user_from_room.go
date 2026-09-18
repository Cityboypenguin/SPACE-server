package room

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RemoveUserFromRoomUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) error
}

var _ RemoveUserFromRoomUseCase = &RemoveUserFromRoomInteractor{}

type RemoveUserFromRoomInteractor struct {
	roomRepo     repository.RoomRepository
	roomUserRepo repository.RoomUserRepository
}

func NewRemoveUserFromRoomUseCase(roomRepo repository.RoomRepository, roomUserRepo repository.RoomUserRepository) RemoveUserFromRoomUseCase {
	return &RemoveUserFromRoomInteractor{roomRepo: roomRepo, roomUserRepo: roomUserRepo}
}

func (uc *RemoveUserFromRoomInteractor) Execute(ctx context.Context, roomID, userID int64) error {
	room, err := uc.roomRepo.GetRoomByID(ctx, roomID)
	if err != nil {
		return err
	}
	if room == nil {
		return errors.New("room not found")
	}
	if room.Type != model.RoomTypeCommunity {
		return errors.New("can only leave community rooms")
	}
	return uc.roomUserRepo.RemoveUserFromRoom(ctx, roomID, userID)
}
