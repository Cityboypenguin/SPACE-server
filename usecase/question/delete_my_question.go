package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteMyQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64) (*model.Question, error)
}

var _ DeleteMyQuestionUseCase = &DeleteMyQuestionInteractor{}

type DeleteMyQuestionInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewDeleteMyQuestionUseCase(questionRepo repository.QuestionRepository) DeleteMyQuestionUseCase {
	return &DeleteMyQuestionInteractor{questionRepo: questionRepo}
}

func (uc *DeleteMyQuestionInteractor) Execute(ctx context.Context, questionID int64) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	q, err := uc.questionRepo.GetQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperr.NotFound("質問が見つかりません")
	}

	ok, err := uc.questionRepo.DeleteQuestionByAsker(ctx, questionID, claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("自分の質問のみ削除できます")
	}

	return q, nil
}
