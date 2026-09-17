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
	mentionStore repository.MessageMentionStore
}

func NewListMentionsByMessageIDsUseCase(mentionStore repository.MessageMentionStore) ListMentionsByMessageIDsUseCase {
	return &ListMentionsByMessageIDsInteractor{mentionStore: mentionStore}
}

func (uc *ListMentionsByMessageIDsInteractor) Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.Mention, error) {
	return uc.mentionStore.ListMentionsByMessageIDs(ctx, messageIDs)
}
