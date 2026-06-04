package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RoomReadStatus struct {
	LastReadAt        *int64
	UnreadCount       int
	PartnerLastReadAt *int64
}

type GetRoomReadStatusUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error)
}

type getRoomReadStatusUseCase struct {
	roomUserRepo repository.RoomUserRepository
	messageRepo  repository.MessageRepository
}

func NewGetRoomReadStatusUseCase(roomUserRepo repository.RoomUserRepository, messageRepo repository.MessageRepository) GetRoomReadStatusUseCase {
	return &getRoomReadStatusUseCase{roomUserRepo: roomUserRepo, messageRepo: messageRepo}
}

func (uc *getRoomReadStatusUseCase) Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error) {
	myLastReadAt, err := uc.roomUserRepo.GetLastReadAt(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}

	var afterTimestamp int64
	if myLastReadAt != nil {
		afterTimestamp = *myLastReadAt
	}
	unreadCount, err := uc.messageRepo.CountUnreadMessages(ctx, roomID, userID, afterTimestamp)
	if err != nil {
		return nil, err
	}

	membersLastReadAt, err := uc.roomUserRepo.GetMembersLastReadAt(ctx, roomID)
	if err != nil {
		return nil, err
	}

	var partnerLastReadAt *int64
	for memberID, readAt := range membersLastReadAt {
		if memberID != userID {
			partnerLastReadAt = readAt
			break
		}
	}

	return &RoomReadStatus{
		LastReadAt:        myLastReadAt,
		UnreadCount:       unreadCount,
		PartnerLastReadAt: partnerLastReadAt,
	}, nil
}
