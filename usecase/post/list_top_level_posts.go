package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListTopLevelPostsUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.Post, int, error)
}

var _ ListTopLevelPostsUseCase = &ListTopLevelPostsInteractor{}

type ListTopLevelPostsInteractor struct {
	postRepo repository.PostLister
}

func NewListTopLevelPostsUseCase(postRepo repository.PostLister) ListTopLevelPostsUseCase {
	return &ListTopLevelPostsInteractor{
		postRepo: postRepo,
	}
}

func (uc *ListTopLevelPostsInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.Post, int, error) {
	return uc.postRepo.ListTopLevelPosts(ctx, q)
}
