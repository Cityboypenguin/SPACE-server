package post

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

type MediaInput struct {
	StorageKey  string
	ContentType string
}

type CreatePostUseCase interface {
	Execute(ctx context.Context, param model.CreatePostParam, mediaInputs []MediaInput) (*model.Post, error)
}

var _ CreatePostUseCase = &CreatePostInteractor{}

type CreatePostInteractor struct {
	postRepo              repository.PostRepository
	mediaRepo             repository.MediaRepository
	txManager             repository.TxManager
	notificationPublisher notificationuc.NotificationPublisher
}

func NewCreatePostUseCase(
	postRepo repository.PostRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
	notificationPublisher notificationuc.NotificationPublisher,
) CreatePostUseCase {
	return &CreatePostInteractor{
		postRepo:              postRepo,
		mediaRepo:             mediaRepo,
		txManager:             txManager,
		notificationPublisher: notificationPublisher,
	}
}

func (uc *CreatePostInteractor) Execute(ctx context.Context, param model.CreatePostParam, mediaInputs []MediaInput) (*model.Post, error) {
	if strings.TrimSpace(param.Content) == "" && len(mediaInputs) == 0 {
		return nil, fmt.Errorf("content cannot be empty")
	}
	if err := validatePostContent(param.Content); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("media/%d/", param.UserID)
	for _, input := range mediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
	}

	post := model.CreatePost(param)

	now := time.Now()
	post.CreatedAt = now
	post.UpdatedAt = now

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		id, err := uc.postRepo.CreatePost(ctx, post)
		if err != nil {
			return err
		}
		post.ID = id

		if tags := ExtractHashtags(post.Content); len(tags) > 0 {
			if err := uc.postRepo.CreatePostHashtags(ctx, post.ID, tags); err != nil {
				return err
			}
		}

		for i, input := range mediaInputs {
			media := &model.Media{
				UploaderUserID: param.UserID,
				StorageKey:     input.StorageKey,
				ContentType:    input.ContentType,
				CreatedAt:      now,
			}
			if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
				return err
			}
			if err := uc.mediaRepo.CreatePostMedia(ctx, post.ID, media.ID, i); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// 返信の場合、親投稿の投稿者へ通知する（自分自身への返信は除く）。
	// 配信失敗は投稿作成の成否に影響させない。
	if uc.notificationPublisher != nil && param.ParentID != nil {
		if parent, perr := uc.postRepo.GetPostByID(ctx, *param.ParentID); perr == nil && parent != nil && parent.UserID != param.UserID {
			targetType := notificationuc.TargetPost
			if err := uc.notificationPublisher.Publish(ctx, notificationuc.PublishParams{
				UserID:     parent.UserID,
				Type:       notificationuc.TypeReply,
				ActorID:    &param.UserID,
				TargetType: &targetType,
				TargetID:   param.ParentID,
				Message:    "あなたの投稿に返信がありました",
			}); err != nil {
				logger.Log.Error().Err(err).Msg("failed to publish reply notification")
			}
		}
	}

	return post, nil
}
