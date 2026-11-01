package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// CountUnansweredQuestionsUseCase はベストアンサーが決まっていない質問を数える。
// 一覧を開く前のチャットタブでもバッジに出すので、一覧とは別の口にしてある。
type CountUnansweredQuestionsUseCase interface {
	Execute(ctx context.Context, roomID int64) (int, error)
}

var _ CountUnansweredQuestionsUseCase = &CountUnansweredQuestionsInteractor{}

type CountUnansweredQuestionsInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewCountUnansweredQuestionsUseCase(questionRepo repository.QuestionRepository) CountUnansweredQuestionsUseCase {
	return &CountUnansweredQuestionsInteractor{questionRepo: questionRepo}
}

func (uc *CountUnansweredQuestionsInteractor) Execute(ctx context.Context, roomID int64) (int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return 0, err
	}
	return uc.questionRepo.CountUnansweredQuestionsByRoomID(ctx, roomID)
}
