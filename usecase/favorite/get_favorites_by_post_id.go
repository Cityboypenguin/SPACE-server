package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFavoritesByPostIDUseCase interface {
	Execute(ctx context.Context, postID int64) ([]*model.Favorite, error)
}

var _ GetFavoritesByPostIDUseCase = &GetFavoritesByPostIDInteractor{}

type GetFavoritesByPostIDInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewGetFavoritesByPostIDUseCase(favoriteRepo repository.FavoriteRepository) GetFavoritesByPostIDUseCase {
	return &GetFavoritesByPostIDInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *GetFavoritesByPostIDInteractor) Execute(ctx context.Context, postID int64) ([]*model.Favorite, error) {
	return uc.favoriteRepo.GetFavoritesByPostID(ctx, postID)
}
