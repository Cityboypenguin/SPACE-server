package question

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64, body string) (*model.Question, error)
}

var _ UpdateQuestionUseCase = &UpdateQuestionInteractor{}

type UpdateQuestionInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewUpdateQuestionUseCase(questionRepo repository.QuestionRepository) UpdateQuestionUseCase {
	return &UpdateQuestionInteractor{questionRepo: questionRepo}
}

func (uc *UpdateQuestionInteractor) Execute(ctx context.Context, questionID int64, body string) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(body) == "" {
		return nil, apperr.InvalidInput("質問本文を入力してください")
	}
	if err := validateBody(body); err != nil {
		return nil, err
	}

	ok, err := uc.questionRepo.UpdateQuestionBody(ctx, questionID, claims.ID, body)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("自分の質問のみ編集できます")
	}

	return uc.questionRepo.GetQuestionByID(ctx, questionID)
}
