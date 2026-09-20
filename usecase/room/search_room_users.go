package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchRoomUsersUseCase interface {
	Execute(ctx context.Context, roomID int64, prefix string, limit int) ([]*model.User, error)
}

type SearchRoomUsersInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewSearchRoomUsersUseCase(roomUserRepo repository.RoomUserRepository) SearchRoomUsersUseCase {
	return &SearchRoomUsersInteractor{roomUserRepo: roomUserRepo}
}

func (uc *SearchRoomUsersInteractor) Execute(ctx context.Context, roomID int64, prefix string, limit int) ([]*model.User, error) {
	return uc.roomUserRepo.SearchRoomUsersByPrefix(ctx, roomID, prefix, limit)
}
