package favoriteuser

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchFavoriteUsersUseCase interface {
	Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.FavoriteUser, error)
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

func (uc *searchFavoritesInteractor) Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.FavoriteUser, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	return uc.favoriteUserRepo.SearchFavoriteUsers(ctx, userID, keyword, q)
}
