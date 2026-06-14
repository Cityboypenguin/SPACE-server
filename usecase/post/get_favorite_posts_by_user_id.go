package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFavoritePostsByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error)
}

var _ GetFavoritePostsByUserIDUseCase = &GetFavoritePostsByUserIDInteractor{}

type GetFavoritePostsByUserIDInteractor struct {
	postRepo repository.PostRepository
}

func NewGetFavoritePostsByUserIDUseCase(postRepo repository.PostRepository) GetFavoritePostsByUserIDUseCase {
	return &GetFavoritePostsByUserIDInteractor{
		postRepo: postRepo,
	}
}

func (uc *GetFavoritePostsByUserIDInteractor) Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error) {
	return uc.postRepo.GetFavoritePostsByUserID(ctx, userID, limit, offset)
}
