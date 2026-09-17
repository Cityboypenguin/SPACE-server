package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListMessagesAroundUseCase は指定メッセージを中心に前後 limit 件ずつ取得する。
// 返信通知からの遷移や引用のタップで、古いメッセージへ直接ジャンプするために使う。
//
// 中心メッセージが削除済み・別ルームの場合は最新ページを返す（呼び出し側は
// ジャンプ先が見つからないだけで、通常どおりルームを開ける）。
type ListMessagesAroundUseCase interface {
	Execute(ctx context.Context, roomID int64, aroundID int64, limit int) (*repository.MessagePage, error)
}

var _ ListMessagesAroundUseCase = &ListMessagesAroundInteractor{}

// 前後を別々に引くため、1件取得(MessageReader)と一覧(MessageReadModel)の
// 両方が要る。合成インターフェースには依存せず、必要な2つだけを受け取る。
type ListMessagesAroundInteractor struct {
	store     repository.MessageReader
	readModel repository.MessageReadModel
}

func NewListMessagesAroundUseCase(store repository.MessageReader, readModel repository.MessageReadModel) ListMessagesAroundUseCase {
	return &ListMessagesAroundInteractor{store: store, readModel: readModel}
}

func (uc *ListMessagesAroundInteractor) Execute(ctx context.Context, roomID int64, aroundID int64, limit int) (*repository.MessagePage, error) {
	center, err := uc.store.GetMessageByID(ctx, aroundID)
	if err != nil {
		return nil, err
	}
	if center == nil || !center.IsInRoom(roomID) {
		return uc.readModel.ListMessagesByRoomID(ctx, repository.MessageQuery{RoomID: roomID, Limit: limit})
	}

	older, err := uc.readModel.ListMessagesByRoomID(ctx, repository.MessageQuery{
		RoomID: roomID,
		Limit:  limit,
		Cursor: repository.MessageCursor{BeforeID: &aroundID},
	})
	if err != nil {
		return nil, err
	}
	newer, err := uc.readModel.ListMessagesByRoomID(ctx, repository.MessageQuery{
		RoomID: roomID,
		Limit:  limit,
		Cursor: repository.MessageCursor{AfterID: &aroundID},
	})
	if err != nil {
		return nil, err
	}

	messages := make([]*model.Message, 0, len(older.Items)+1+len(newer.Items))
	messages = append(messages, older.Items...)
	messages = append(messages, center)
	messages = append(messages, newer.Items...)
	// 前半ページの「さらに古い側」と後半ページの「さらに新しい側」を合わせたものが、
	// 中心を挟んだこのページ全体の前後の有無になる。
	return &repository.MessagePage{
		Items:         messages,
		HasMoreBefore: older.HasMoreBefore,
		HasMoreAfter:  newer.HasMoreAfter,
	}, nil
}
