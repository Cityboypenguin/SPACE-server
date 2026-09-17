package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRoomReadStatusBatchUseCase interface {
	// roomIDs は「同じ種別のルームの一覧」を渡すこと（DM 一覧・コミュニティ一覧の
	// ように、呼び出し元は必ず一種類の一覧を描画するためこれで足りる）。roomType は
	// PartnerLastReadAt を返してよいか（＝DM か）の判定にだけ使う。
	// 1件ずつの取得は GetRoomReadStatusUseCase。
	Execute(ctx context.Context, roomIDs []int64, userID int64, roomType string) (map[int64]*RoomReadStatus, error)
}

type getRoomReadStatusBatchUseCase struct {
	roomUserRepo  repository.RoomUserRepository
	unreadCounter repository.MessageUnreadCounter
}

func NewGetRoomReadStatusBatchUseCase(roomUserRepo repository.RoomUserRepository, unreadCounter repository.MessageUnreadCounter) GetRoomReadStatusBatchUseCase {
	return &getRoomReadStatusBatchUseCase{roomUserRepo: roomUserRepo, unreadCounter: unreadCounter}
}

func (uc *getRoomReadStatusBatchUseCase) Execute(ctx context.Context, roomIDs []int64, userID int64, roomType string) (map[int64]*RoomReadStatus, error) {
	if len(roomIDs) == 0 {
		return map[int64]*RoomReadStatus{}, nil
	}

	// 一覧でも既読位置のメッセージIDまで持ち帰る。一覧から開いたルームと room クエリで
	// 開いたルームとで lastReadMessageID の有無が変わると、未読ページの起点が経路に
	// よって ID / 時刻に分かれてしまうため。
	myPositions, err := uc.roomUserRepo.GetLastReadByRoomIDs(ctx, userID, roomIDs)
	if err != nil {
		return nil, err
	}

	unreadCountMap, err := uc.unreadCounter.CountUnreadMessagesByRoomIDs(ctx, userID, roomIDs)
	if err != nil {
		return nil, err
	}

	// 相手の既読位置を持つのは DM だけなので、コミュニティ一覧ではこのクエリを
	// 流さない（理由は RoomReadStatus.PartnerLastReadAt のコメント参照）。
	var membersLastReadAtMap map[int64]map[int64]*int64
	if roomType == model.RoomTypeDM {
		membersLastReadAtMap, err = uc.roomUserRepo.GetMembersLastReadAtByRoomIDs(ctx, roomIDs)
		if err != nil {
			return nil, err
		}
	}

	result := make(map[int64]*RoomReadStatus, len(roomIDs))
	for _, roomID := range roomIDs {
		status := &RoomReadStatus{
			UnreadCount:       unreadCountMap[roomID],
			PartnerLastReadAt: partnerLastReadAt(membersLastReadAtMap[roomID], userID),
		}
		if position := myPositions[roomID]; position != nil {
			status.LastReadAt = position.LastReadAt
			status.LastReadMessageID = position.LastReadMessageID
		}
		result[roomID] = status
	}
	return result, nil
}
