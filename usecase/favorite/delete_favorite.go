package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteFavoriteUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ DeleteFavoriteUseCase = &DeleteFavoriteInteractor{}

type DeleteFavoriteInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewDeleteFavoriteUseCase(favoriteRepo repository.FavoriteRepository) DeleteFavoriteUseCase {
	return &DeleteFavoriteInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *DeleteFavoriteInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	return uc.favoriteRepo.DeleteFavorite(ctx, id)
}
