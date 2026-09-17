package room

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MarkRoomAsReadUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) error
}

type markRoomAsReadUseCase struct {
	roomUserRepo  repository.RoomUserRepository
	messageReader repository.MessageReadModel
}

func NewMarkRoomAsReadUseCase(roomUserRepo repository.RoomUserRepository, messageReader repository.MessageReadModel) MarkRoomAsReadUseCase {
	return &markRoomAsReadUseCase{roomUserRepo: roomUserRepo, messageReader: messageReader}
}

func (uc *markRoomAsReadUseCase) Execute(ctx context.Context, roomID, userID int64) error {
	lastReadMessageID, err := resolveReadMessageID(ctx, uc.messageReader, roomID)
	if err != nil {
		return err
	}
	return uc.roomUserRepo.UpdateLastRead(ctx, roomID, userID, lastReadMessageID, time.Now().Unix())
}

// resolveReadMessageID は既読位置として保存する「そのルームの最新メッセージID」を引く。
//
// 既読位置は時刻ではなくメッセージIDで持つ（同じ秒に既読更新と新着が起きたときの
// 取りこぼしを避けるため。理由は repository.ReadPosition のコメント）。その ID を
// クライアントから受け取る形にすると GraphQL スキーマ（markRoomAsRead）の変更が要るので、
// サーバ側で解決する。メッセージが1件も無いルームでは nil を返し、既読時刻だけが進む。
func resolveReadMessageID(ctx context.Context, reader repository.MessageReadModel, roomID int64) (*int64, error) {
	return reader.GetLatestMessageID(ctx, roomID)
}
