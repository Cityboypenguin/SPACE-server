package favoriteuser

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFavoriteUsersByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64) ([]*model.FavoriteUser, error)
}

var _ GetFavoriteUsersByUserIDUseCase = &getFavoriteUsersByUserIDInteractor{}

type getFavoriteUsersByUserIDInteractor struct {
	favoriteUserRepo repository.FavoriteUserRepository
}

func NewGetFavoriteUsersByUserIDUseCase(favoriteUserRepo repository.FavoriteUserRepository) GetFavoriteUsersByUserIDUseCase {
	return &getFavoriteUsersByUserIDInteractor{
		favoriteUserRepo: favoriteUserRepo,
	}
}

func (uc *getFavoriteUsersByUserIDInteractor) Execute(ctx context.Context, userID int64) ([]*model.FavoriteUser, error) {
	return uc.favoriteUserRepo.GetFavoriteUsersByUserID(ctx, userID)
}
