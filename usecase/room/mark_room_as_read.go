package room

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MarkRoomAsReadUseCase interface {
	// lastReadMessageID はクライアントが「実際に画面に出した最後のメッセージ」。
	// nil のときの扱いは resolveReadMessageID のコメント参照。
	Execute(ctx context.Context, roomID, userID int64, lastReadMessageID *int64) error
}

type markRoomAsReadUseCase struct {
	roomUserRepo  repository.ReadPositionRepository
	messageReader repository.MessageReadModel
}

func NewMarkRoomAsReadUseCase(roomUserRepo repository.ReadPositionRepository, messageReader repository.MessageReadModel) MarkRoomAsReadUseCase {
	return &markRoomAsReadUseCase{roomUserRepo: roomUserRepo, messageReader: messageReader}
}

func (uc *markRoomAsReadUseCase) Execute(ctx context.Context, roomID, userID int64, lastReadMessageID *int64) error {
	resolved, err := resolveReadMessageID(ctx, uc.messageReader, roomID, lastReadMessageID)
	if err != nil {
		return err
	}
	return uc.roomUserRepo.UpdateLastRead(ctx, roomID, userID, resolved, time.Now().Unix())
}

// resolveReadMessageID は既読位置として保存するメッセージIDを決める。
//
// 既読位置は時刻ではなくメッセージIDで持つ（同じ秒に既読更新と新着が起きたときの
// 取りこぼしを避けるため。理由は repository.ReadPosition のコメント）。
//
// requested がある場合（＝クライアントが「ここまで表示した」と申告した場合）:
// そのIDが本当にこのルームのメッセージかを必ず確かめる。検証を省くと、他ルームの
// 大きいIDを渡すだけでそのルームの未読を丸ごと消せてしまう（未読判定は
// m.id > last_read_message_id の単純比較で、IDの出所を見ていないため）。
// 不正な値は書き込まずに INVALID_INPUT で拒否する。「黙って最新へ倒す」ような
// 救済はしない: 壊れた値を送ってくるクライアントに気づけなくなるうえ、
// 表示していないメッセージまで既読にする副作用が残る。
//
// requested が nil の場合（古いクライアント互換）:
// markRoomAsRead に lastReadMessageID 引数が無かった頃のクライアントは何も渡して
// こないので、従来どおりサーバ側で「その時点の最新メッセージID」を既読位置にする。
// この経路には「クライアントがまだ描画していない新着まで既読になる」という
// 取りこぼしが残るが、引数を付けた新しいクライアントは必ず渡してくるので、
// 移行が済めばこの分岐に入るのは古い端末だけになる。
//
// どちらの経路でも、メッセージが1件も無いルームでは nil を返し既読時刻だけが進む。
// 巻き戻しの防止（GREATEST）はリポジトリ側の責務なので、ここでは「進めたい位置」を
// 決めるだけで、渡ってきたIDが現在の位置より古いかどうかは見ない。
func resolveReadMessageID(ctx context.Context, reader repository.MessageReadModel, roomID int64, requested *int64) (*int64, error) {
	if requested == nil {
		return reader.GetLatestMessageID(ctx, roomID)
	}

	exists, err := reader.MessageExistsInRoom(ctx, roomID, *requested)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, apperr.InvalidInput("lastReadMessageID does not belong to this room")
	}
	return requested, nil
}
