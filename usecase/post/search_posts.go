package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchPostsUseCase interface {
	Execute(ctx context.Context, query string, q repository.PageQuery) ([]*model.Post, error)
}

var _ SearchPostsUseCase = &SearchPostsInteractor{}

type SearchPostsInteractor struct {
	postRepo repository.PostSearcher
}

func NewSearchPostsUseCase(postRepo repository.PostSearcher) SearchPostsUseCase {
	return &SearchPostsInteractor{
		postRepo: postRepo,
	}
}

func (uc *SearchPostsInteractor) Execute(ctx context.Context, query string, q repository.PageQuery) ([]*model.Post, error) {
	posts, err := uc.postRepo.SearchPosts(ctx, query, q)
	if err != nil {
		return nil, err
	}

	return posts, nil
}
