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
	postRepo     repository.PostRepository
}

func NewCreateFavoriteUseCase(favoriteRepo repository.FavoriteRepository, postRepo repository.PostRepository) CreateFavoriteUseCase {
	return &CreateFavoriteInteractor{
		favoriteRepo: favoriteRepo,
		postRepo:     postRepo,
	}
}

func (uc *CreateFavoriteInteractor) Execute(ctx context.Context, param model.CreateFavoriteParam) (*model.Favorite, error) {
	favorite := model.CreateFavorite(param)

	_, err := uc.favoriteRepo.CreateFavorite(ctx, favorite)
	if err != nil {
		return nil, err
	}

	return favorite, nil
}
