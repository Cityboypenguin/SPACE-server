package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SelectBestAnswerUseCase interface {
	Execute(ctx context.Context, questionID, answerID int64) (*model.Question, error)
}

var _ SelectBestAnswerUseCase = &SelectBestAnswerInteractor{}

type SelectBestAnswerInteractor struct {
	questionRepo repository.QuestionRepository
	answerRepo   repository.AnswerRepository
}

func NewSelectBestAnswerUseCase(questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository) SelectBestAnswerUseCase {
	return &SelectBestAnswerInteractor{questionRepo: questionRepo, answerRepo: answerRepo}
}

// Execute lets the asker mark one of the answers to their question as the best
// answer, setting isAnswered=true (F-04-2: 質問者がベストアンサーを選ぶ).
func (uc *SelectBestAnswerInteractor) Execute(ctx context.Context, questionID, answerID int64) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	answer, err := uc.answerRepo.GetAnswerByID(ctx, answerID)
	if err != nil {
		return nil, err
	}
	if answer == nil || answer.QuestionID != questionID {
		return nil, apperr.InvalidInput("指定された回答はこの質問のものではありません")
	}

	ok, err := uc.questionRepo.SetBestAnswer(ctx, questionID, answerID, claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("質問者のみがベストアンサーを選択できます")
	}

	return uc.questionRepo.GetQuestionByID(ctx, questionID)
}
