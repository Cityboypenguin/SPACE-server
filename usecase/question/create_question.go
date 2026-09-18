package question

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

const MaxMediaCount = 4

type CreateQuestionUseCase interface {
	Execute(ctx context.Context, roomID int64, body string, mediaInputs []model.MediaInput) (*model.Question, error)
}

var _ CreateQuestionUseCase = &CreateQuestionInteractor{}

type CreateQuestionInteractor struct {
	questionRepo    repository.QuestionRepository
	mediaRepo       repository.MediaRepository
	txManager       repository.TxManager
	requireWritable course.RequireWritableCourseRoomUseCase
	anonIdentity    anonusecase.GetOrCreateAnonymousIdentityUseCase
}

func NewCreateQuestionUseCase(
	questionRepo repository.QuestionRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
	requireWritable course.RequireWritableCourseRoomUseCase,
	anonIdentity anonusecase.GetOrCreateAnonymousIdentityUseCase,
) CreateQuestionUseCase {
	return &CreateQuestionInteractor{
		questionRepo:    questionRepo,
		mediaRepo:       mediaRepo,
		txManager:       txManager,
		requireWritable: requireWritable,
		anonIdentity:    anonIdentity,
	}
}

func (uc *CreateQuestionInteractor) Execute(ctx context.Context, roomID int64, body string, mediaInputs []model.MediaInput) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if len(mediaInputs) > MaxMediaCount {
		return nil, apperr.InvalidInput(fmt.Sprintf("写真は%d枚まで添付できます", MaxMediaCount))
	}
	if err := validateBody(body); err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("media/%d/", claims.ID)
	for _, input := range mediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
	}
	if _, err := uc.requireWritable.Execute(ctx, roomID); err != nil {
		return nil, err
	}

	// 匿名ID(匿名NNN)は投稿時に確定させる。質問箱は授業内チャット専用なので、
	// メッセージ送信（usecase/chat）と同じ扱いにして「番号は初投稿順」という
	// 仕様を全ての投稿経路で守る。表示側は採番しない（読むだけ）。
	// 詳細は usecase/chat の ensureAnonymousIdentity のコメントを参照。
	if _, err := uc.anonIdentity.Execute(ctx, roomID, claims.ID); err != nil {
		return nil, err
	}

	q := &model.Question{
		RoomID:      roomID,
		AskerUserID: claims.ID,
		AuthorRole:  model.AuthorRoleStudent,
		Body:        body,
	}

	now := time.Now()
	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.questionRepo.SaveQuestion(ctx, q); err != nil {
			return err
		}
		// 添付はまとめて保存する（media 行を1本、紐付けを1本）。以前は入力1件ごとに
		// 2往復していたので、4枚付けると8往復していた。
		if medias := model.NewMediaBatch(claims.ID, mediaInputs, now); len(medias) > 0 {
			if err := uc.mediaRepo.CreateMediaBatch(ctx, medias); err != nil {
				return err
			}
			if err := uc.mediaRepo.CreateQuestionMediaBatch(ctx, q.ID, model.MediaIDs(medias), 0); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return q, nil
}
