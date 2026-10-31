package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListMessageReactionsByMessageIDsUseCase は複数メッセージのリアクションを
// まとめて引く（DataLoader 用）。「自分が付けたか」は呼び出し元のユーザーで解決する
// ので認証必須。リアクションが無いメッセージは map に入らない。
type ListMessageReactionsByMessageIDsUseCase interface {
	Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.MessageReaction, error)
}

var _ ListMessageReactionsByMessageIDsUseCase = &ListMessageReactionsByMessageIDsInteractor{}

type ListMessageReactionsByMessageIDsInteractor struct {
	reactions repository.MessageReactionRepository
}

func NewListMessageReactionsByMessageIDsUseCase(reactions repository.MessageReactionRepository) ListMessageReactionsByMessageIDsUseCase {
	return &ListMessageReactionsByMessageIDsInteractor{reactions: reactions}
}

func (uc *ListMessageReactionsByMessageIDsInteractor) Execute(ctx context.Context, messageIDs []int64) (map[int64][]*model.MessageReaction, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.reactions.ListMessageReactionsByMessageIDs(ctx, messageIDs, claims.ID)
}
