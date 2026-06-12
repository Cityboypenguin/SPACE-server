package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListTopLevelPostsUseCase interface {
	Execute(ctx context.Context, limit, offset int) ([]*model.Post, int, error)
}

var _ ListTopLevelPostsUseCase = &ListTopLevelPostsInteractor{}

type ListTopLevelPostsInteractor struct {
	postRepo repository.PostRepository
}

func NewListTopLevelPostsUseCase(postRepo repository.PostRepository) ListTopLevelPostsUseCase {
	return &ListTopLevelPostsInteractor{
		postRepo: postRepo,
	}
}

func (uc *ListTopLevelPostsInteractor) Execute(ctx context.Context, limit, offset int) ([]*model.Post, int, error) {
	return uc.postRepo.ListTopLevelPosts(ctx, limit, offset)
}
