package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListAnswersUseCase interface {
	Execute(ctx context.Context, questionID int64, limit, offset int) ([]*repository.AnswerWithLikes, int, error)
}

var _ ListAnswersUseCase = &ListAnswersInteractor{}

type ListAnswersInteractor struct {
	answerRepo repository.AnswerRepository
}

func NewListAnswersUseCase(answerRepo repository.AnswerRepository) ListAnswersUseCase {
	return &ListAnswersInteractor{answerRepo: answerRepo}
}

// Execute returns a page of questionID's answers ordered by like count descending
// (F-04-2 いいねの多い回答を上に表示), ties broken by createdAt ascending, along with
// the total answer count for pagination.
func (uc *ListAnswersInteractor) Execute(ctx context.Context, questionID int64, limit, offset int) ([]*repository.AnswerWithLikes, int, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, 0, err
	}
	items, err := uc.answerRepo.ListAnswersWithLikesByQuestionID(ctx, questionID, claims.ID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.answerRepo.CountAnswersByQuestionID(ctx, questionID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
