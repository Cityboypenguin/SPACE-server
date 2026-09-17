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
	unreadCounter repository.MessageUnreadCounter
}

func NewCountUnreadByRoomTypeUseCase(unreadCounter repository.MessageUnreadCounter) CountUnreadByRoomTypeUseCase {
	return &countUnreadByRoomTypeInteractor{unreadCounter: unreadCounter}
}

func (uc *countUnreadByRoomTypeInteractor) Execute(ctx context.Context, userID int64, roomType string) (int, error) {
	return uc.unreadCounter.CountUnreadMessagesByRoomType(ctx, userID, roomType)
}
