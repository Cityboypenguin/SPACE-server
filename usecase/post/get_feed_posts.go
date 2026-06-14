package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFeedPostsUseCase interface {
	Execute(ctx context.Context, viewerID int64, limit, offset int) ([]*model.Post, int, error)
}

var _ GetFeedPostsUseCase = &GetFeedPostsInteractor{}

type GetFeedPostsInteractor struct {
	postRepo repository.PostRepository
}

func NewGetFeedPostsUseCase(postRepo repository.PostRepository) GetFeedPostsUseCase {
	return &GetFeedPostsInteractor{postRepo: postRepo}
}

func (uc *GetFeedPostsInteractor) Execute(ctx context.Context, viewerID int64, limit, offset int) ([]*model.Post, int, error) {
	return uc.postRepo.GetFeedPosts(ctx, viewerID, limit, offset)
}
