package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRoomReadStatusBatchUseCase interface {
	Execute(ctx context.Context, roomIDs []int64, userID int64) (map[int64]*RoomReadStatus, error)
}

type getRoomReadStatusBatchUseCase struct {
	roomUserRepo repository.RoomUserRepository
	messageRepo  repository.MessageRepository
}

func NewGetRoomReadStatusBatchUseCase(roomUserRepo repository.RoomUserRepository, messageRepo repository.MessageRepository) GetRoomReadStatusBatchUseCase {
	return &getRoomReadStatusBatchUseCase{roomUserRepo: roomUserRepo, messageRepo: messageRepo}
}

func (uc *getRoomReadStatusBatchUseCase) Execute(ctx context.Context, roomIDs []int64, userID int64) (map[int64]*RoomReadStatus, error) {
	if len(roomIDs) == 0 {
		return map[int64]*RoomReadStatus{}, nil
	}

	myLastReadAtMap, err := uc.roomUserRepo.GetLastReadAtByRoomIDs(ctx, userID, roomIDs)
	if err != nil {
		return nil, err
	}

	unreadCountMap, err := uc.messageRepo.CountUnreadMessagesByRoomIDs(ctx, userID, roomIDs)
	if err != nil {
		return nil, err
	}

	membersLastReadAtMap, err := uc.roomUserRepo.GetMembersLastReadAtByRoomIDs(ctx, roomIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]*RoomReadStatus, len(roomIDs))
	for _, roomID := range roomIDs {
		var partnerLastReadAt *int64
		for memberID, readAt := range membersLastReadAtMap[roomID] {
			if memberID != userID {
				partnerLastReadAt = readAt
				break
			}
		}

		result[roomID] = &RoomReadStatus{
			LastReadAt:        myLastReadAtMap[roomID],
			UnreadCount:       unreadCountMap[roomID],
			PartnerLastReadAt: partnerLastReadAt,
		}
	}
	return result, nil
}
