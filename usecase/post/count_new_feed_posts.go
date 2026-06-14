package post

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CountNewFeedPostsUseCase interface {
	Execute(ctx context.Context, viewerID int64, since time.Time) (int, error)
}

var _ CountNewFeedPostsUseCase = &CountNewFeedPostsInteractor{}

type CountNewFeedPostsInteractor struct {
	postRepo repository.PostRepository
}

func NewCountNewFeedPostsUseCase(postRepo repository.PostRepository) CountNewFeedPostsUseCase {
	return &CountNewFeedPostsInteractor{postRepo: postRepo}
}

func (uc *CountNewFeedPostsInteractor) Execute(ctx context.Context, viewerID int64, since time.Time) (int, error) {
	return uc.postRepo.CountNewFeedPosts(ctx, viewerID, since)
}
