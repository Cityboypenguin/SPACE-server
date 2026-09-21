package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type LikeAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error)
}

var _ LikeAnswerUseCase = &LikeAnswerInteractor{}

type LikeAnswerInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	answerRepo      repository.AnswerRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewLikeAnswerUseCase(events EventPublisher, questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository, requireWritable course.RequireWritableCourseRoomUseCase) LikeAnswerUseCase {
	return &LikeAnswerInteractor{events: orNoop(events), questionRepo: questionRepo, answerRepo: answerRepo, requireWritable: requireWritable}
}

func (uc *LikeAnswerInteractor) Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if _, err := findAnswerInWritableRoom(ctx, uc.questionRepo, uc.answerRepo, uc.requireWritable, answerID); err != nil {
		return nil, err
	}

	if err := uc.answerRepo.LikeAnswer(ctx, answerID, claims.ID); err != nil {
		return nil, err
	}
	updated, err := uc.answerRepo.GetAnswerWithLikesByID(ctx, answerID, claims.ID)
	if err != nil {
		return nil, err
	}
	// 配信はここで出す（usecase/answer/events.go 参照）。
	uc.events.AnswerUpdated(ctx, updated.Answer)
	return updated, nil
}

type UnlikeAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error)
}

var _ UnlikeAnswerUseCase = &UnlikeAnswerInteractor{}

type UnlikeAnswerInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	answerRepo      repository.AnswerRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewUnlikeAnswerUseCase(events EventPublisher, questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository, requireWritable course.RequireWritableCourseRoomUseCase) UnlikeAnswerUseCase {
	return &UnlikeAnswerInteractor{events: orNoop(events), questionRepo: questionRepo, answerRepo: answerRepo, requireWritable: requireWritable}
}

func (uc *UnlikeAnswerInteractor) Execute(ctx context.Context, answerID int64) (*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if _, err := findAnswerInWritableRoom(ctx, uc.questionRepo, uc.answerRepo, uc.requireWritable, answerID); err != nil {
		return nil, err
	}

	if err := uc.answerRepo.UnlikeAnswer(ctx, answerID, claims.ID); err != nil {
		return nil, err
	}
	updated, err := uc.answerRepo.GetAnswerWithLikesByID(ctx, answerID, claims.ID)
	if err != nil {
		return nil, err
	}
	// 配信はここで出す（usecase/answer/events.go 参照）。
	uc.events.AnswerUpdated(ctx, updated.Answer)
	return updated, nil
}

// findAnswerInWritableRoom loads answerID and rejects the operation unless the
// course room its question belongs to is currently writable for the caller
// (F-06: 履修をやめた授業・終了した学期では閲覧のみ).
func findAnswerInWritableRoom(ctx context.Context, questionRepo repository.QuestionRepository, answerRepo repository.AnswerRepository, requireWritable course.RequireWritableCourseRoomUseCase, answerID int64) (*model.Answer, error) {
	a, err := answerRepo.GetAnswerByID(ctx, answerID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, apperr.NotFound("回答が見つかりません")
	}

	q, err := questionRepo.GetQuestionByID(ctx, a.QuestionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperr.NotFound("質問が見つかりません")
	}
	if _, err := requireWritable.Execute(ctx, q.RoomID); err != nil {
		return nil, err
	}
	return a, nil
}
