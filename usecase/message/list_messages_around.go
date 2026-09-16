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
	Execute(ctx context.Context, roomID int64, aroundID int64, limit int) ([]*model.Message, bool, bool, error)
}

var _ ListMessagesAroundUseCase = &ListMessagesAroundInteractor{}

type ListMessagesAroundInteractor struct {
	messageRepo repository.MessageRepository
}

func NewListMessagesAroundUseCase(messageRepo repository.MessageRepository) ListMessagesAroundUseCase {
	return &ListMessagesAroundInteractor{messageRepo: messageRepo}
}

func (uc *ListMessagesAroundInteractor) Execute(ctx context.Context, roomID int64, aroundID int64, limit int) ([]*model.Message, bool, bool, error) {
	center, err := uc.messageRepo.GetMessageByID(ctx, aroundID)
	if err != nil {
		return nil, false, false, err
	}
	if center == nil || center.RoomID != roomID {
		return uc.messageRepo.ListMessagesByRoomID(ctx, roomID, limit, nil, nil, nil)
	}

	older, hasMoreBefore, _, err := uc.messageRepo.ListMessagesByRoomID(ctx, roomID, limit, &aroundID, nil, nil)
	if err != nil {
		return nil, false, false, err
	}
	newer, _, hasMoreAfter, err := uc.messageRepo.ListMessagesByRoomID(ctx, roomID, limit, nil, &aroundID, nil)
	if err != nil {
		return nil, false, false, err
	}

	messages := make([]*model.Message, 0, len(older)+1+len(newer))
	messages = append(messages, older...)
	messages = append(messages, center)
	messages = append(messages, newer...)
	return messages, hasMoreBefore, hasMoreAfter, nil
}
