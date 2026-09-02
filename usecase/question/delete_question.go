package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64) (bool, error)
}

var _ DeleteQuestionUseCase = &DeleteQuestionInteractor{}

type DeleteQuestionInteractor struct {
	questionRepo repository.QuestionRepository
}

func NewDeleteQuestionUseCase(questionRepo repository.QuestionRepository) DeleteQuestionUseCase {
	return &DeleteQuestionInteractor{questionRepo: questionRepo}
}

// Execute removes a question and its answers. Authorization (admin-only, for
// moderating 授業内チャット's 質問箱) is enforced by the caller.
func (uc *DeleteQuestionInteractor) Execute(ctx context.Context, questionID int64) (bool, error) {
	return uc.questionRepo.DeleteQuestion(ctx, questionID)
}
