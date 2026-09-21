package favoriteuser

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListFavoriteUsersUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.FavoriteUser, int, error)
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

func (uc *listFavoritesInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.FavoriteUser, int, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return uc.favoriteUserRepo.ListFavoriteUsers(ctx, userID, q)
}
