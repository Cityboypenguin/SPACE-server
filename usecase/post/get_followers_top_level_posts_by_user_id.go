package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFollowersTopLevelPostsByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error)
}

var _ GetFollowersTopLevelPostsByUserIDUseCase = &GetFollowersTopLevelPostsByUserIDInteractor{}

type GetFollowersTopLevelPostsByUserIDInteractor struct {
	postRepo repository.PostRepository
}

func NewGetFollowersTopLevelPostsByUserIDUseCase(postRepo repository.PostRepository) GetFollowersTopLevelPostsByUserIDUseCase {
	return &GetFollowersTopLevelPostsByUserIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetFollowersTopLevelPostsByUserIDInteractor) Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error) {
	return uc.postRepo.GetfollowersTopLevelPostsByUserID(ctx, userID, limit, offset)
}
