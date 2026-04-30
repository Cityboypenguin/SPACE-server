package post

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdatePostUseCase interface {
	Execute(ctx context.Context, id int64, param model.UpdatePostParam) (*model.Post, error)
}

var _ UpdatePostUseCase = &UpdatePostInteractor{}

type UpdatePostInteractor struct {
	postRepo repository.PostRepository
}

func NewUpdatePostUseCase(postRepo repository.PostRepository) UpdatePostUseCase {
	return &UpdatePostInteractor{
		postRepo: postRepo,
	}
}

func (uc *UpdatePostInteractor) Execute(ctx context.Context, id int64, param model.UpdatePostParam) (*model.Post, error) {
	post, err := uc.postRepo.GetPostByID(ctx, id)
	if err != nil {
		return nil, err
	}

	post.UpdatePost(param)

	now := time.Now()
	post.UpdatedAt = now

	err = uc.postRepo.UpdatePost(ctx, post)
	if err != nil {
		return nil, err
	}

	return post, nil
}
