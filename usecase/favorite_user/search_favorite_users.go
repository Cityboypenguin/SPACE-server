package favoriteuser

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchFavoriteUsersUseCase interface {
	Execute(ctx context.Context, userID int64, keyword string) ([]*model.FavoriteUser, error)
}

var _ SearchFavoriteUsersUseCase = &searchFavoritesInteractor{}

type searchFavoritesInteractor struct {
	favoriteUserRepo repository.FavoriteUserRepository
}

func NewSearchFavoriteUsersUseCase(favoriteUserRepo repository.FavoriteUserRepository) SearchFavoriteUsersUseCase {
	return &searchFavoritesInteractor{
		favoriteUserRepo: favoriteUserRepo,
	}
}

func (uc *searchFavoritesInteractor) Execute(ctx context.Context, userID int64, keyword string) ([]*model.FavoriteUser, error) {
	return uc.favoriteUserRepo.SearchFavoriteUsers(ctx, userID, keyword)
}
