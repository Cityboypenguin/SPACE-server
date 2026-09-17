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
	roomUserRepo  repository.RoomUserRepository
	unreadCounter repository.MessageUnreadCounter
}

func NewGetRoomReadStatusUseCase(roomUserRepo repository.RoomUserRepository, unreadCounter repository.MessageUnreadCounter) GetRoomReadStatusUseCase {
	return &getRoomReadStatusUseCase{roomUserRepo: roomUserRepo, unreadCounter: unreadCounter}
}

// Execute は DM・コミュニティの既読位置と未読数を返す。
//
// 未読の起点は repository.UnreadOrigin の規則どおり「既読メッセージID → 既読時刻 →
// 全件」。通常ルームにはフォールバックの時刻が無い（＝まだ一度も読んでいなければ
// 他人のメッセージは全部未読）ので、NewUnreadOrigin には nil を渡す。
func (uc *getRoomReadStatusUseCase) Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error) {
	position, err := uc.roomUserRepo.GetLastRead(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}

	unreadCount, err := uc.unreadCounter.CountUnreadMessages(ctx, roomID, userID, repository.NewUnreadOrigin(position, nil))
	if err != nil {
		return nil, err
	}

	var myLastReadAt *int64
	if position != nil {
		myLastReadAt = position.LastReadAt
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
