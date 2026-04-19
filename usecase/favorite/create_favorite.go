package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateFavoriteUseCase interface {
	Execute(ctx context.Context, param model.CreateFavoriteParam) (*model.Favorite, error)
}

var _ CreateFavoriteUseCase = &CreateFavoriteInteractor{}

type CreateFavoriteInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewCreateFavoriteUseCase(favoriteRepo repository.FavoriteRepository) CreateFavoriteUseCase {
	return &CreateFavoriteInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *CreateFavoriteInteractor) Execute(ctx context.Context, param model.CreateFavoriteParam) (*model.Favorite, error) {
	favorite := &model.Favorite{}
	favorite.CreateFavorite(param)

	err := uc.favoriteRepo.CreateFavorite(ctx, favorite)
	if err != nil {
		return nil, err
	}

	return favorite, nil
}
