package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListFavoritesUseCase interface {
	Execute(ctx context.Context) ([]*model.Favorite, error)
}

var _ ListFavoritesUseCase = &ListFavoritesInteractor{}

type ListFavoritesInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewListFavoritesUseCase(favoriteRepo repository.FavoriteRepository) ListFavoritesUseCase {
	return &ListFavoritesInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *ListFavoritesInteractor) Execute(ctx context.Context) ([]*model.Favorite, error) {
	return uc.favoriteRepo.ListFavorites(ctx)
}
