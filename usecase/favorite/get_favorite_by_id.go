package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFavoriteByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Favorite, error)
}

var _ GetFavoriteByIDUseCase = &GetFavoriteByIDInteractor{}

type GetFavoriteByIDInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewGetFavoriteByIDUseCase(favoriteRepo repository.FavoriteRepository) GetFavoriteByIDUseCase {
	return &GetFavoriteByIDInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *GetFavoriteByIDInteractor) Execute(ctx context.Context, id int64) (*model.Favorite, error) {
	return uc.favoriteRepo.GetFavoriteByID(ctx, id)
}
