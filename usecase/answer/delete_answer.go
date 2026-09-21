package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type DeleteAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64) (*model.Answer, error)
}

var _ DeleteAnswerUseCase = &DeleteAnswerInteractor{}

type DeleteAnswerInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	answerRepo      repository.AnswerRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewDeleteAnswerUseCase(events EventPublisher, questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository, requireWritable course.RequireWritableCourseRoomUseCase) DeleteAnswerUseCase {
	return &DeleteAnswerInteractor{events: orNoop(events), questionRepo: questionRepo, answerRepo: answerRepo, requireWritable: requireWritable}
}

// Execute lets the answer's author delete it, unless it is currently selected as
// the question's best answer. It returns the answer as it was just before
// deletion, so the caller can notify subscribers.
//
// 編集と同じく、書き込みできない授業（過去の学期、または時間割に未登録）では削除もできない。
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
	if q != nil {
		if _, err := uc.requireWritable.Execute(ctx, q.RoomID); err != nil {
			return nil, err
		}
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

	// 配信はここで出す。リゾルバの手順にしておくと、リゾルバを通らない経路では
	// 購読中の画面が動かない（usecase/*/events.go 参照）。
	uc.events.AnswerDeleted(ctx, a)
	return a, nil
}
