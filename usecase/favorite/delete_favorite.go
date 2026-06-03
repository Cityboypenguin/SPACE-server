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

func NewDeleteFavoriteUseCase(favoriteRepo repository.FavoriteRepository, postRepo repository.PostRepository) DeleteFavoriteUseCase {
	return &DeleteFavoriteInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *DeleteFavoriteInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	_, err := uc.favoriteRepo.GetFavoriteByID(ctx, id)
	if err != nil {
		return false, err
	}

	deleted, err := uc.favoriteRepo.DeleteFavorite(ctx, id)
	if err != nil {
		return false, err
	}

	return deleted, nil
}
