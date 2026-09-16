package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListMentionsByMessageIDsUseCase はメッセージIDごとのメンション一覧をまとめて引く。
// チャットは1画面に数十件並ぶため、DataLoader から1クエリで解決する。
type ListMentionsByMessageIDsUseCase interface {
	Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Mention, error)
}

var _ ListMentionsByMessageIDsUseCase = &ListMentionsByMessageIDsInteractor{}

type ListMentionsByMessageIDsInteractor struct {
	messageRepo repository.MessageRepository
}

func NewListMentionsByMessageIDsUseCase(messageRepo repository.MessageRepository) ListMentionsByMessageIDsUseCase {
	return &ListMentionsByMessageIDsInteractor{messageRepo: messageRepo}
}

func (uc *ListMentionsByMessageIDsInteractor) Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Mention, error) {
	return uc.messageRepo.ListMentionsByMessageIDs(ctx, messageIDs)
}
