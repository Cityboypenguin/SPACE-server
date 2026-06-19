package post

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeletePostUseCase interface {
	Execute(ctx context.Context, id int64, requesterID int64, allowAnyOwner bool) (bool, error)
}

var _ DeletePostUseCase = &DeletePostInteractor{}

type DeletePostInteractor struct {
	postRepo repository.PostRepository
}

func NewDeletePostUseCase(postRepo repository.PostRepository) DeletePostUseCase {
	return &DeletePostInteractor{
		postRepo: postRepo,
	}
}

func (uc *DeletePostInteractor) Execute(ctx context.Context, id int64, requesterID int64, allowAnyOwner bool) (bool, error) {
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
