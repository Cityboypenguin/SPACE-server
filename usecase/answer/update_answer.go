package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64, body string) (*repository.AnswerWithLikes, error)
}

var _ UpdateAnswerUseCase = &UpdateAnswerInteractor{}

type UpdateAnswerInteractor struct {
	questionRepo repository.QuestionRepository
	answerRepo   repository.AnswerRepository
}

func NewUpdateAnswerUseCase(questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository) UpdateAnswerUseCase {
	return &UpdateAnswerInteractor{questionRepo: questionRepo, answerRepo: answerRepo}
}

// Execute lets the answer's author edit its body, unless it is currently selected
// as the question's best answer (locked to keep the accepted answer stable once
// chosen).
func (uc *UpdateAnswerInteractor) Execute(ctx context.Context, answerID int64, body string) (*repository.AnswerWithLikes, error) {
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
		return nil, apperr.Forbidden("自分の回答のみ編集できます")
	}

	q, err := uc.questionRepo.GetQuestionByID(ctx, a.QuestionID)
	if err != nil {
		return nil, err
	}
	if q != nil && q.BestAnswerID != nil && *q.BestAnswerID == answerID {
		return nil, apperr.InvalidInput("ベストアンサーに選ばれている回答は編集できません")
	}

	ok, err := uc.answerRepo.UpdateAnswerBody(ctx, answerID, claims.ID, body)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("自分の回答のみ編集できます")
	}

	return uc.answerRepo.GetAnswerWithLikesByID(ctx, answerID, claims.ID)
}
