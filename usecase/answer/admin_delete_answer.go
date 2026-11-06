package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
)

type AdminDeleteAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64) (bool, error)
}

type AdminDeleteAnswerInteractor struct {
	events         EventPublisher
	answerRepo     repository.AnswerRepository
	questionRepo   repository.QuestionRepository
	questionEvents questionusecase.EventPublisher
}

func NewAdminDeleteAnswerUseCase(events EventPublisher, questionEvents questionusecase.EventPublisher, answerRepo repository.AnswerRepository, questionRepo repository.QuestionRepository) AdminDeleteAnswerUseCase {
	return &AdminDeleteAnswerInteractor{events: orNoop(events), answerRepo: answerRepo, questionRepo: questionRepo, questionEvents: questionEvents}
}

// Execute removes a single answer as moderation, including in archived courses.
// Author deletion retains its separate ownership and best-answer restrictions.
func (uc *AdminDeleteAnswerInteractor) Execute(ctx context.Context, answerID int64) (bool, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return false, err
	}
	a, err := uc.answerRepo.GetAnswerByID(ctx, answerID)
	if err != nil || a == nil {
		return false, err
	}
	ok, err := uc.answerRepo.AdminDeleteAnswer(ctx, answerID)
	if err != nil || !ok {
		return false, err
	}
	uc.events.AnswerDeleted(ctx, a)
	// 削除でベストアンサーや件数が変わった質問も、開いている画面へ反映する。
	q, err := uc.questionRepo.GetQuestionByID(ctx, a.QuestionID)
	if err != nil {
		logger.Log.Error().Err(err).Int64("question_id", a.QuestionID).Msg("failed to reload question after answer deletion")
	} else if q != nil && uc.questionEvents != nil {
		uc.questionEvents.QuestionUpdated(ctx, q)
	}
	return true, nil
}
