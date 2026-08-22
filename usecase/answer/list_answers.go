package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListAnswersUseCase interface {
	Execute(ctx context.Context, questionID int64) ([]*model.Answer, error)
}

var _ ListAnswersUseCase = &ListAnswersInteractor{}

type ListAnswersInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewListAnswersUseCase(answerRepo repository.AnswerRepository) ListAnswersUseCase {
	return &ListAnswersInteractor{answerRepo: answerRepo}
}

func (uc *ListAnswersInteractor) Execute(ctx context.Context, questionID int64) ([]*model.Answer, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.answerRepo.ListAnswersByQuestionID(ctx, questionID)
}
