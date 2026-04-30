package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchPostsUseCase interface {
	Execute(ctx context.Context, query string) ([]*model.Post, error)
}

var _ SearchPostsUseCase = &SearchPostsInteractor{}

type SearchPostsInteractor struct {
	postRepo repository.PostRepository
}

func NewSearchPostsUseCase(postRepo repository.PostRepository) SearchPostsUseCase {
	return &SearchPostsInteractor{
		postRepo: postRepo,
	}
}

func (uc *SearchPostsInteractor) Execute(ctx context.Context, query string) ([]*model.Post, error) {
	return uc.postRepo.SearchPosts(ctx, query)
}
