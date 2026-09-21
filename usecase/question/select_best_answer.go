package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type SelectBestAnswerUseCase interface {
	Execute(ctx context.Context, questionID, answerID int64) (*model.Question, error)
}

var _ SelectBestAnswerUseCase = &SelectBestAnswerInteractor{}

type SelectBestAnswerInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	answerRepo      repository.AnswerRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewSelectBestAnswerUseCase(events EventPublisher, questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository, requireWritable course.RequireWritableCourseRoomUseCase) SelectBestAnswerUseCase {
	return &SelectBestAnswerInteractor{events: orNoop(events), questionRepo: questionRepo, answerRepo: answerRepo, requireWritable: requireWritable}
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
	if err := requireWritableQuestionRoom(ctx, uc.questionRepo, uc.requireWritable, questionID); err != nil {
		return nil, err
	}

	ok, err := uc.questionRepo.SetBestAnswer(ctx, questionID, answerID, claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("質問者のみがベストアンサーを選択できます")
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
