package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFavoritesByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Favorite, error)
}

var _ GetFavoritesByPostIDsUseCase = &GetFavoritesByPostIDsInteractor{}

type GetFavoritesByPostIDsInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewGetFavoritesByPostIDsUseCase(favoriteRepo repository.FavoriteRepository) GetFavoritesByPostIDsUseCase {
	return &GetFavoritesByPostIDsInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *GetFavoritesByPostIDsInteractor) Execute(ctx context.Context, postIDs []int64) (map[int64][]*model.Favorite, error) {
	return uc.favoriteRepo.GetFavoritesByPostIDs(ctx, postIDs)
}
