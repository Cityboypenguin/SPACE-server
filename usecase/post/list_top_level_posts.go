package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListTopLevelPostsUseCase interface {
	Execute(ctx context.Context) ([]*model.Post, error)
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

func (uc *ListTopLevelPostsInteractor) Execute(ctx context.Context) ([]*model.Post, error) {
	posts, err := uc.postRepo.ListTopLevelPosts(ctx)
	if err != nil {
		return nil, err
	}

	return posts, nil
}
