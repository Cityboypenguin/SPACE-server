package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type CancelBestAnswerUseCase interface {
	Execute(ctx context.Context, questionID int64) (*model.Question, error)
}

var _ CancelBestAnswerUseCase = &CancelBestAnswerInteractor{}

type CancelBestAnswerInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewCancelBestAnswerUseCase(events EventPublisher, questionRepo repository.QuestionRepository, requireWritable course.RequireWritableCourseRoomUseCase) CancelBestAnswerUseCase {
	return &CancelBestAnswerInteractor{events: orNoop(events), questionRepo: questionRepo, requireWritable: requireWritable}
}

// Execute lets the asker undo a previously selected best answer, clearing
// isAnswered/bestAnswerID so the question returns to an open state.
func (uc *CancelBestAnswerInteractor) Execute(ctx context.Context, questionID int64) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireWritableQuestionRoom(ctx, uc.questionRepo, uc.requireWritable, questionID); err != nil {
		return nil, err
	}

	ok, err := uc.questionRepo.ClearBestAnswer(ctx, questionID, claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("質問者のみがベストアンサーを取り消せます")
	}

	updated, err := uc.questionRepo.GetQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	// 配信はここで出す。リゾルバの手順にしておくと、リゾルバを通らない経路では
	// 購読中の画面が動かない（usecase/*/events.go 参照）。
	uc.events.QuestionUpdated(ctx, updated)
	return updated, nil
}
