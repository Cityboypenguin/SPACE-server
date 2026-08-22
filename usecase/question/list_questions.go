package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListQuestionsUseCase interface {
	Execute(ctx context.Context, roomID int64, limit, offset int) ([]*model.Question, int, error)
}

var _ ListQuestionsUseCase = &ListQuestionsInteractor{}

type ListQuestionsInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewListQuestionsUseCase(questionRepo repository.QuestionRepository) ListQuestionsUseCase {
	return &ListQuestionsInteractor{questionRepo: questionRepo}
}

func (uc *ListQuestionsInteractor) Execute(ctx context.Context, roomID int64, limit, offset int) ([]*model.Question, int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, 0, err
	}
	return uc.questionRepo.ListQuestionsByRoomID(ctx, roomID, limit, offset)
}
