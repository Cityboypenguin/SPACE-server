package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetMessagesByIDsUseCase は複数メッセージをIDでまとめて引く。引用返信の返信先を
// DataLoader 経由で N+1 なく解決するために使う。削除済み・存在しないIDは
// 結果の map に含まれない。
type GetMessagesByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*model.Message, error)
}

var _ GetMessagesByIDsUseCase = &GetMessagesByIDsInteractor{}

type GetMessagesByIDsInteractor struct {
	messageRepo repository.MessageRepository
}

func NewGetMessagesByIDsUseCase(messageRepo repository.MessageRepository) GetMessagesByIDsUseCase {
	return &GetMessagesByIDsInteractor{messageRepo: messageRepo}
}

func (uc *GetMessagesByIDsInteractor) Execute(ctx context.Context, ids []int64) (map[int64]*model.Message, error) {
	return uc.messageRepo.GetMessagesByIDs(ctx, ids)
}
