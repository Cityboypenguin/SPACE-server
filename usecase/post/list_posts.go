package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListPostsUseCase interface {
	Execute(ctx context.Context) ([]*model.Post, error)
}

var _ ListPostsUseCase = &ListPostsInteractor{}

type ListPostsInteractor struct {
	postRepo repository.PostRepository
}

func NewListPostsUseCase(postRepo repository.PostRepository) ListPostsUseCase {
	return &ListPostsInteractor{
		postRepo: postRepo,
	}
}

func (uc *ListPostsInteractor) Execute(ctx context.Context) ([]*model.Post, error) {
	return uc.postRepo.ListPosts(ctx)
}
