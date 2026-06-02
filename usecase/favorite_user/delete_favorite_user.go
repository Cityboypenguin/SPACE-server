package favoriteuser

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteFavoriteUserUseCase interface {
	Execute(ctx context.Context, userID, favoriteID int64) (bool, error)
}

var _ DeleteFavoriteUserUseCase = &deleteFavoriteUserInteractor{}

type deleteFavoriteUserInteractor struct {
	favoriteUserRepo repository.FavoriteUserRepository
}

func NewDeleteFavoriteUserUseCase(favoriteUserRepo repository.FavoriteUserRepository) DeleteFavoriteUserUseCase {
	return &deleteFavoriteUserInteractor{
		favoriteUserRepo: favoriteUserRepo,
	}
}

func (uc *deleteFavoriteUserInteractor) Execute(ctx context.Context, userID, favoriteID int64) (bool, error) {
	favoriteUser := &model.FavoriteUser{
		UserID:         userID,
		FavoriteUserID: favoriteID,
	}

	return uc.favoriteUserRepo.DeleteFavoriteUser(ctx, favoriteUser.UserID, favoriteUser.FavoriteUserID)
}
