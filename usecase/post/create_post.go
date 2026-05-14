package post

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
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
	postRepo  repository.PostRepository
	mediaRepo repository.MediaRepository
	txManager repository.TxManager
}

func NewCreatePostUseCase(
	postRepo repository.PostRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
) CreatePostUseCase {
	return &CreatePostInteractor{
		postRepo:  postRepo,
		mediaRepo: mediaRepo,
		txManager: txManager,
	}
}

func (uc *CreatePostInteractor) Execute(ctx context.Context, param model.CreatePostParam, mediaInputs []MediaInput) (*model.Post, error) {
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

	return post, nil
}
