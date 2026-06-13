package favoriteuser

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListFavoriteUsersUseCase interface {
	Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error)
}

var _ ListFavoriteUsersUseCase = &listFavoritesInteractor{}

type listFavoritesInteractor struct {
	favoriteUserRepo repository.FavoriteUserRepository
}

func NewListFavoriteUsersUseCase(favoriteUserRepo repository.FavoriteUserRepository) ListFavoriteUsersUseCase {
	return &listFavoritesInteractor{
		favoriteUserRepo: favoriteUserRepo,
	}
}

func (uc *listFavoritesInteractor) Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error) {
	return uc.favoriteUserRepo.ListFavoriteUsers(ctx, userID, limit, offset)
}
