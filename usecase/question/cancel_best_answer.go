package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CancelBestAnswerUseCase interface {
	Execute(ctx context.Context, questionID int64) (*model.Question, error)
}

var _ CancelBestAnswerUseCase = &CancelBestAnswerInteractor{}

type CancelBestAnswerInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewCancelBestAnswerUseCase(questionRepo repository.QuestionRepository) CancelBestAnswerUseCase {
	return &CancelBestAnswerInteractor{questionRepo: questionRepo}
}

// Execute lets the asker undo a previously selected best answer, clearing
// isAnswered/bestAnswerID so the question returns to an open state.
func (uc *CancelBestAnswerInteractor) Execute(ctx context.Context, questionID int64) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	ok, err := uc.questionRepo.ClearBestAnswer(ctx, questionID, claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("質問者のみがベストアンサーを取り消せます")
	}

	return uc.questionRepo.GetQuestionByID(ctx, questionID)
}
