package post

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdatePostUseCase interface {
	Execute(ctx context.Context, param model.UpdatePostParam, newMediaInputs []MediaInput, deletedMediaIDs []int64) (*model.Post, error)
}

var _ UpdatePostUseCase = &UpdatePostInteractor{}

type UpdatePostInteractor struct {
	postRepo  repository.PostRepository
	mediaRepo repository.MediaRepository
	txManager repository.TxManager
}

func NewUpdatePostUseCase(postRepo repository.PostRepository, mediaRepo repository.MediaRepository, txManager repository.TxManager) *UpdatePostInteractor {
	return &UpdatePostInteractor{
		postRepo:  postRepo,
		mediaRepo: mediaRepo,
		txManager: txManager,
	}
}

func (uc *UpdatePostInteractor) Execute(ctx context.Context, param model.UpdatePostParam, newMediaInputs []MediaInput, deletedMediaIDs []int64) (*model.Post, error) {
	if param.Content != nil {
		if err := validatePostContent(*param.Content); err != nil {
			return nil, err
		}
	}

	prefix := fmt.Sprintf("media/%d/", param.UserID)
	for _, input := range newMediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
	}

	post, err := uc.postRepo.GetPostByID(ctx, param.PostID)
	if err != nil {
		return nil, err
	}
	if post == nil || post.UserID != param.UserID {
		return nil, fmt.Errorf("post not found or unauthorized")
	}

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {

		post.UpdatePost(param)

		if err := uc.postRepo.UpdatePost(ctx, post); err != nil {
			return err
		}

		// 本文が更新された場合はハッシュタグを再同期する。
		if param.Content != nil {
			if err := uc.postRepo.DeletePostHashtagsByPostID(ctx, post.ID); err != nil {
				return err
			}
			if tags := ExtractHashtags(post.Content); len(tags) > 0 {
				if err := uc.postRepo.CreatePostHashtags(ctx, post.ID, tags); err != nil {
					return err
				}
			}
		}

		for _, mediaID := range deletedMediaIDs {
			if err := uc.mediaRepo.DeleteMediaByIDAndUserID(ctx, mediaID, param.UserID); err != nil {
				return err
			}
		}

		if len(newMediaInputs) > 0 {
			currentMaxPos, err := uc.mediaRepo.GetMaxPostMediaPosition(ctx, param.PostID)
			if err != nil {
				return err
			}

			for i, input := range newMediaInputs {
				media := &model.Media{
					UploaderUserID: param.UserID,
					StorageKey:     input.StorageKey,
					ContentType:    input.ContentType,
					CreatedAt:      post.UpdatedAt,
				}

				if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
					return err
				}

				newPosition := currentMaxPos + 1 + i

				if err := uc.mediaRepo.CreatePostMedia(ctx, post.ID, media.ID, newPosition); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return post, nil
}
