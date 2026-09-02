package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64) (*model.Answer, error)
}

var _ DeleteAnswerUseCase = &DeleteAnswerInteractor{}

type DeleteAnswerInteractor struct {
	questionRepo repository.QuestionRepository
	answerRepo   repository.AnswerRepository
}

func NewDeleteAnswerUseCase(questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository) DeleteAnswerUseCase {
	return &DeleteAnswerInteractor{questionRepo: questionRepo, answerRepo: answerRepo}
}

// Execute lets the answer's author delete it, unless it is currently selected as
// the question's best answer. It returns the answer as it was just before
// deletion, so the caller can notify subscribers.
func (uc *DeleteAnswerInteractor) Execute(ctx context.Context, answerID int64) (*model.Answer, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	a, err := uc.answerRepo.GetAnswerByID(ctx, answerID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, apperr.NotFound("回答が見つかりません")
	}
	if a.AuthorUserID != claims.ID {
		return nil, apperr.Forbidden("自分の回答のみ削除できます")
	}

	q, err := uc.questionRepo.GetQuestionByID(ctx, a.QuestionID)
	if err != nil {
		return nil, err
	}
	if q != nil && q.BestAnswerID != nil && *q.BestAnswerID == answerID {
		return nil, apperr.InvalidInput("ベストアンサーに選ばれている回答は削除できません")
	}

	ok, err := uc.answerRepo.DeleteAnswer(ctx, answerID, claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("自分の回答のみ削除できます")
	}

	return a, nil
}
