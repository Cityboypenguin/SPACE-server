package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFollowersTopLevelPostsByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.Post, int, error)
}

var _ GetFollowersTopLevelPostsByUserIDUseCase = &GetFollowersTopLevelPostsByUserIDInteractor{}

type GetFollowersTopLevelPostsByUserIDInteractor struct {
	postRepo repository.PostLister
}

func NewGetFollowersTopLevelPostsByUserIDUseCase(postRepo repository.PostLister) GetFollowersTopLevelPostsByUserIDUseCase {
	return &GetFollowersTopLevelPostsByUserIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetFollowersTopLevelPostsByUserIDInteractor) Execute(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.Post, int, error) {
	return uc.postRepo.GetfollowersTopLevelPostsByUserID(ctx, userID, q)
}
