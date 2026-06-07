package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetMembersUnreadCountsUseCase interface {
	Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error)
}

type getMembersUnreadCountsUseCase struct {
	messageRepo repository.MessageRepository
}

func NewGetMembersUnreadCountsUseCase(_ repository.RoomUserRepository, messageRepo repository.MessageRepository) GetMembersUnreadCountsUseCase {
	return &getMembersUnreadCountsUseCase{messageRepo: messageRepo}
}

func (uc *getMembersUnreadCountsUseCase) Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	return uc.messageRepo.CountUnreadMessagesPerMember(ctx, roomID, excludeUserID)
}
