package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CountUnreadByRoomTypeUseCase interface {
	Execute(ctx context.Context, userID int64, roomType string) (int, error)
}

var _ CountUnreadByRoomTypeUseCase = &countUnreadByRoomTypeInteractor{}

type countUnreadByRoomTypeInteractor struct {
	messageRepo repository.MessageRepository
}

func NewCountUnreadByRoomTypeUseCase(messageRepo repository.MessageRepository) CountUnreadByRoomTypeUseCase {
	return &countUnreadByRoomTypeInteractor{messageRepo: messageRepo}
}

func (uc *countUnreadByRoomTypeInteractor) Execute(ctx context.Context, userID int64, roomType string) (int, error) {
	return uc.messageRepo.CountUnreadMessagesByRoomType(ctx, userID, roomType)
}
