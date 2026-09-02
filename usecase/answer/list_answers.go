package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListAnswersUseCase interface {
	Execute(ctx context.Context, questionID int64) ([]*repository.AnswerWithLikes, error)
}

var _ ListAnswersUseCase = &ListAnswersInteractor{}

type ListAnswersInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewListAnswersUseCase(answerRepo repository.AnswerRepository) ListAnswersUseCase {
	return &ListAnswersInteractor{answerRepo: answerRepo}
}

// Execute returns questionID's answers ordered by like count descending (F-04-2
// いいねの多い回答を上に表示), ties broken by createdAt ascending.
func (uc *ListAnswersInteractor) Execute(ctx context.Context, questionID int64) ([]*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.answerRepo.ListAnswersWithLikesByQuestionID(ctx, questionID, claims.ID)
}
