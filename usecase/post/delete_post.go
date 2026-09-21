package post

import (
	"context"

	"fmt"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
)

type DeletePostUseCase interface {
	Execute(ctx context.Context, id int64, allowAnyOwner bool) (bool, error)
}

var _ DeletePostUseCase = &DeletePostInteractor{}

type DeletePostInteractor struct {
	postRepo postDeleteRepository
}

func NewDeletePostUseCase(postRepo postDeleteRepository) DeletePostUseCase {
	return &DeletePostInteractor{
		postRepo: postRepo,
	}
}

func (uc *DeletePostInteractor) Execute(ctx context.Context, id int64, allowAnyOwner bool) (bool, error) {
	requesterID, err := authz.CallerID(ctx)
	if err != nil {
		return false, err
	}
	post, err := uc.postRepo.GetPostByID(ctx, id)
	if err != nil {
		return false, err
	}
	if post == nil {
		return false, fmt.Errorf("post not found")
	}
	if !allowAnyOwner && post.UserID != requesterID {
		return false, fmt.Errorf("forbidden: can only delete your own posts")
	}
	return uc.postRepo.DeletePost(ctx, id)
}
