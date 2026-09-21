package answer

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type UpdateAnswerUseCase interface {
	Execute(ctx context.Context, answerID int64, body string, deletedMediaIDs []int64) (*repository.AnswerWithLikes, error)
}

var _ UpdateAnswerUseCase = &UpdateAnswerInteractor{}

type UpdateAnswerInteractor struct {
	events          EventPublisher
	questionRepo    repository.QuestionRepository
	answerRepo      repository.AnswerRepository
	mediaRepo       mediaReplaceRepository
	txManager       repository.TxManager
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewUpdateAnswerUseCase(events EventPublisher,
	questionRepo repository.QuestionRepository,
	answerRepo repository.AnswerRepository,
	mediaRepo mediaReplaceRepository,
	txManager repository.TxManager,
	requireWritable course.RequireWritableCourseRoomUseCase,
) UpdateAnswerUseCase {
	return &UpdateAnswerInteractor{
		events:          orNoop(events),
		questionRepo:    questionRepo,
		answerRepo:      answerRepo,
		mediaRepo:       mediaRepo,
		txManager:       txManager,
		requireWritable: requireWritable,
	}
}

// Execute lets the answer's author edit its body and remove photos attached to it,
// unless it is currently selected as the question's best answer (locked to keep the
// accepted answer stable once chosen) or the course room is no longer writable for
// them (F-06). deletedMediaIDs must all be attached to this answer, and the body may
// be empty only while at least one photo remains (the same rule as when posting).
func (uc *UpdateAnswerInteractor) Execute(ctx context.Context, answerID int64, body string, deletedMediaIDs []int64) (*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBody(body); err != nil {
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
		return nil, apperr.Forbidden("自分の回答のみ編集できます")
	}

	q, err := uc.questionRepo.GetQuestionByID(ctx, a.QuestionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperr.NotFound("質問が見つかりません")
	}
	if _, err := uc.requireWritable.Execute(ctx, q.RoomID); err != nil {
		return nil, err
	}
	if q.BestAnswerID != nil && *q.BestAnswerID == answerID {
		return nil, apperr.InvalidInput("ベストアンサーに選ばれている回答は編集できません")
	}

	mediaByAnswer, err := uc.mediaRepo.ListByAnswerIDs(ctx, []int64{answerID})
	if err != nil {
		return nil, err
	}
	attached := make(map[int64]struct{}, len(mediaByAnswer[answerID]))
	for _, m := range mediaByAnswer[answerID] {
		attached[m.ID] = struct{}{}
	}
	deleting := make(map[int64]struct{}, len(deletedMediaIDs))
	for _, id := range deletedMediaIDs {
		if _, ok := attached[id]; !ok {
			return nil, apperr.InvalidInput("この回答に添付されていない写真は削除できません")
		}
		deleting[id] = struct{}{}
	}
	if strings.TrimSpace(body) == "" && len(deleting) == len(attached) {
		return nil, apperr.InvalidInput("回答本文を入力するか、写真を1枚以上残してください")
	}

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		ok, err := uc.answerRepo.UpdateAnswerBody(ctx, answerID, claims.ID, body)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Forbidden("自分の回答のみ編集できます")
		}
		for id := range deleting {
			if err := uc.mediaRepo.DeleteAnswerMedia(ctx, answerID, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	updated, err := uc.answerRepo.GetAnswerWithLikesByID(ctx, answerID, claims.ID)
	if err != nil {
		return nil, err
	}
	// 配信はここで出す。リゾルバの手順にしておくと、リゾルバを通らない経路では
	// 購読中の画面が動かない（usecase/*/events.go 参照）。
	uc.events.AnswerUpdated(ctx, updated.Answer)
	return updated, nil
}
