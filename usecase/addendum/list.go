package addendum

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListQuestionAddendaUseCase / ListAnswerAddendaUseCase は複数の質問・回答の追記を
// まとめて引く（DataLoader 用）。追記が無いものは map に入らない。
type ListQuestionAddendaUseCase interface {
	Execute(ctx context.Context, questionIDs []int64) (map[int64][]*model.Addendum, error)
}

type ListAnswerAddendaUseCase interface {
	Execute(ctx context.Context, answerIDs []int64) (map[int64][]*model.Addendum, error)
}

type listQuestionAddenda struct{ repo repository.AddendumRepository }

func NewListQuestionAddendaUseCase(repo repository.AddendumRepository) ListQuestionAddendaUseCase {
	return &listQuestionAddenda{repo: repo}
}

func (uc *listQuestionAddenda) Execute(ctx context.Context, ids []int64) (map[int64][]*model.Addendum, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.repo.ListQuestionAddenda(ctx, ids)
}

type listAnswerAddenda struct{ repo repository.AddendumRepository }

func NewListAnswerAddendaUseCase(repo repository.AddendumRepository) ListAnswerAddendaUseCase {
	return &listAnswerAddenda{repo: repo}
}

func (uc *listAnswerAddenda) Execute(ctx context.Context, ids []int64) (map[int64][]*model.Addendum, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.repo.ListAnswerAddenda(ctx, ids)
}
