package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetMembersUnreadCountsUseCase interface {
	Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error)
}

type getMembersUnreadCountsUseCase struct {
	unreadCounter repository.MessageUnreadCounter
}

func NewGetMembersUnreadCountsUseCase(_ repository.RoomUserRepository, unreadCounter repository.MessageUnreadCounter) GetMembersUnreadCountsUseCase {
	return &getMembersUnreadCountsUseCase{unreadCounter: unreadCounter}
}

func (uc *getMembersUnreadCountsUseCase) Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	return uc.unreadCounter.CountUnreadMessagesPerMember(ctx, roomID, excludeUserID)
}
