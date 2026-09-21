package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type DeleteMyQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64) (*model.Question, error)
}

var _ DeleteMyQuestionUseCase = &DeleteMyQuestionInteractor{}

type DeleteMyQuestionInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewDeleteMyQuestionUseCase(events EventPublisher, questionRepo repository.QuestionRepository, requireWritable course.RequireWritableCourseRoomUseCase) DeleteMyQuestionUseCase {
	return &DeleteMyQuestionInteractor{events: orNoop(events), questionRepo: questionRepo, requireWritable: requireWritable}
}

// Execute lets the asker delete their own question, as long as the course room is
// still writable for them (F-06: 履修をやめた授業・終了した学期では閲覧のみ) — the
// same rule as editing. 管理者による削除は AdminDeleteQuestion(DeleteQuestionUseCase)
// が担うため、ここでは考慮しない。
func (uc *DeleteMyQuestionInteractor) Execute(ctx context.Context, questionID int64) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	q, err := uc.questionRepo.GetQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperr.NotFound("質問が見つかりません")
	}
	if q.AskerUserID != claims.ID {
		return nil, apperr.Forbidden("自分の質問のみ削除できます")
	}
	if _, err := uc.requireWritable.Execute(ctx, q.RoomID); err != nil {
		return nil, err
	}

	ok, err := uc.questionRepo.DeleteQuestionByAsker(ctx, questionID, claims.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.Forbidden("自分の質問のみ削除できます")
	}

	// 配信はここで出す。リゾルバの手順にしておくと、リゾルバを通らない経路では
	// 購読中の画面が動かない（usecase/*/events.go 参照）。
	uc.events.QuestionDeleted(ctx, q)
	return q, nil
}
