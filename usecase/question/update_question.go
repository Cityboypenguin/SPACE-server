package question

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type UpdateQuestionUseCase interface {
	Execute(ctx context.Context, questionID int64, body string, deletedMediaIDs []int64) (*model.Question, error)
}

var _ UpdateQuestionUseCase = &UpdateQuestionInteractor{}

type UpdateQuestionInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	mediaRepo       mediaReplaceRepository
	txManager       repository.TxManager
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewUpdateQuestionUseCase(events EventPublisher,
	questionRepo repository.QuestionRepository,
	mediaRepo mediaReplaceRepository,
	txManager repository.TxManager,
	requireWritable course.RequireWritableCourseRoomUseCase,
) UpdateQuestionUseCase {
	return &UpdateQuestionInteractor{
		events:          orNoop(events),
		questionRepo:    questionRepo,
		mediaRepo:       mediaRepo,
		txManager:       txManager,
		requireWritable: requireWritable,
	}
}

// Execute lets the asker edit their question's body and remove photos attached to
// it, as long as the course room is still writable for them (F-06: 履修をやめた授業・
// 終了した学期では閲覧のみ). deletedMediaIDs must all be attached to this question.
// The body may be empty only while at least one photo remains, the same rule as
// when posting.
func (uc *UpdateQuestionInteractor) Execute(ctx context.Context, questionID int64, body string, deletedMediaIDs []int64) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBody(body); err != nil {
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
		return nil, apperr.Forbidden("自分の質問のみ編集できます")
	}
	if _, err := uc.requireWritable.Execute(ctx, q.RoomID); err != nil {
		return nil, err
	}

	mediaByQuestion, err := uc.mediaRepo.ListByQuestionIDs(ctx, []int64{questionID})
	if err != nil {
		return nil, err
	}
	attached := make(map[int64]struct{}, len(mediaByQuestion[questionID]))
	for _, m := range mediaByQuestion[questionID] {
		attached[m.ID] = struct{}{}
	}
	deleting := make(map[int64]struct{}, len(deletedMediaIDs))
	for _, id := range deletedMediaIDs {
		if _, ok := attached[id]; !ok {
			return nil, apperr.InvalidInput("この質問に添付されていない写真は削除できません")
		}
		deleting[id] = struct{}{}
	}
	if strings.TrimSpace(body) == "" && len(deleting) == len(attached) {
		return nil, apperr.InvalidInput("質問本文を入力するか、写真を1枚以上残してください")
	}

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		ok, err := uc.questionRepo.UpdateQuestionBody(ctx, questionID, claims.ID, body)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Forbidden("自分の質問のみ編集できます")
		}
		for id := range deleting {
			if err := uc.mediaRepo.DeleteQuestionMedia(ctx, questionID, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
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

// requireWritableQuestionRoom rejects the operation unless the course room that
// questionID belongs to is currently writable for the caller.
func requireWritableQuestionRoom(ctx context.Context, questionRepo repository.QuestionRepository, requireWritable course.RequireWritableCourseRoomUseCase, questionID int64) error {
	q, err := questionRepo.GetQuestionByID(ctx, questionID)
	if err != nil {
		return err
	}
	if q == nil {
		return apperr.NotFound("質問が見つかりません")
	}
	_, err = requireWritable.Execute(ctx, q.RoomID)
	return err
}
