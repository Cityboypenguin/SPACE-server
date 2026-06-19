package favoriteuser

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListFollowersUseCase interface {
	Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error)
}

var _ ListFollowersUseCase = &listFollowersInteractor{}

type listFollowersInteractor struct {
	favoriteUserRepo repository.FavoriteUserRepository
}

func NewListFollowersUseCase(favoriteUserRepo repository.FavoriteUserRepository) ListFollowersUseCase {
	return &listFollowersInteractor{
		favoriteUserRepo: favoriteUserRepo,
	}
}

func (uc *listFollowersInteractor) Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error) {
	return uc.favoriteUserRepo.ListFollowers(ctx, userID, limit, offset)
}
