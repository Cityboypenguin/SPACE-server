package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFeedPostsUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.Post, int, error)
}

var _ GetFeedPostsUseCase = &GetFeedPostsInteractor{}

type GetFeedPostsInteractor struct {
	postRepo repository.PostLister
}

func NewGetFeedPostsUseCase(postRepo repository.PostLister) GetFeedPostsUseCase {
	return &GetFeedPostsInteractor{postRepo: postRepo}
}

func (uc *GetFeedPostsInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.Post, int, error) {
	viewerID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return uc.postRepo.GetFeedPosts(ctx, viewerID, q)
}
