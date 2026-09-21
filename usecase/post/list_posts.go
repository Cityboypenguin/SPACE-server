package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListPostsUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.Post, int, error)
}

var _ ListPostsUseCase = &ListPostsInteractor{}

type ListPostsInteractor struct {
	postRepo repository.PostLister
}

func NewListPostsUseCase(postRepo repository.PostLister) ListPostsUseCase {
	return &ListPostsInteractor{
		postRepo: postRepo,
	}
}

func (uc *ListPostsInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.Post, int, error) {
	return uc.postRepo.ListPosts(ctx, q)
}
