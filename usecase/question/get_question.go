package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetQuestionByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Question, error)
}

var _ GetQuestionByIDUseCase = &GetQuestionByIDInteractor{}

type GetQuestionByIDInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewGetQuestionByIDUseCase(questionRepo repository.QuestionRepository) GetQuestionByIDUseCase {
	return &GetQuestionByIDInteractor{questionRepo: questionRepo}
}

func (uc *GetQuestionByIDInteractor) Execute(ctx context.Context, id int64) (*model.Question, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.questionRepo.GetQuestionByID(ctx, id)
}
