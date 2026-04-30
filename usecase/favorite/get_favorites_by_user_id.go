package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFavoritesByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64) ([]*model.Favorite, error)
}

var _ GetFavoritesByUserIDUseCase = &GetFavoritesByUserIDInteractor{}

type GetFavoritesByUserIDInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewGetFavoritesByUserIDUseCase(favoriteRepo repository.FavoriteRepository) GetFavoritesByUserIDUseCase {
	return &GetFavoritesByUserIDInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *GetFavoritesByUserIDInteractor) Execute(ctx context.Context, userID int64) ([]*model.Favorite, error) {
	return uc.favoriteRepo.GetFavoritesByUserID(ctx, userID)
}
