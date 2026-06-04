package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetMembersUnreadCountsUseCase interface {
	Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error)
}

type getMembersUnreadCountsUseCase struct {
	roomUserRepo repository.RoomUserRepository
	messageRepo  repository.MessageRepository
}

func NewGetMembersUnreadCountsUseCase(roomUserRepo repository.RoomUserRepository, messageRepo repository.MessageRepository) GetMembersUnreadCountsUseCase {
	return &getMembersUnreadCountsUseCase{roomUserRepo: roomUserRepo, messageRepo: messageRepo}
}

func (uc *getMembersUnreadCountsUseCase) Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	membersLastReadAt, err := uc.roomUserRepo.GetMembersLastReadAt(ctx, roomID)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]int)
	for memberID, lastReadAt := range membersLastReadAt {
		if memberID == excludeUserID {
			continue
		}
		var afterTimestamp int64
		if lastReadAt != nil {
			afterTimestamp = *lastReadAt
		}
		count, err := uc.messageRepo.CountUnreadMessages(ctx, roomID, memberID, afterTimestamp)
		if err != nil {
			continue
		}
		result[memberID] = count
	}
	return result, nil
}
