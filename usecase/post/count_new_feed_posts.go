package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CountNewFeedPostsUseCase interface {
	Execute(ctx context.Context, since time.Time) (int, error)
}

var _ CountNewFeedPostsUseCase = &CountNewFeedPostsInteractor{}

type CountNewFeedPostsInteractor struct {
	postRepo repository.PostLister
}

func NewCountNewFeedPostsUseCase(postRepo repository.PostLister) CountNewFeedPostsUseCase {
	return &CountNewFeedPostsInteractor{postRepo: postRepo}
}

func (uc *CountNewFeedPostsInteractor) Execute(ctx context.Context, since time.Time) (int, error) {
	viewerID, err := authz.CallerID(ctx)
	if err != nil {
		return 0, err
	}
	return uc.postRepo.CountNewFeedPosts(ctx, viewerID, since)
}
