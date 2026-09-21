package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteFavoriteByUserIDAndPostIDUseCase interface {
	Execute(ctx context.Context, post_id int64) (bool, error)
}

var _ DeleteFavoriteByUserIDAndPostIDUseCase = &DeleteFavoriteByUserIDAndPostIDInteractor{}

type DeleteFavoriteByUserIDAndPostIDInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewDeleteFavoriteByUserIDAndPostIDUseCase(favoriteRepo repository.FavoriteRepository) DeleteFavoriteByUserIDAndPostIDUseCase {
	return &DeleteFavoriteByUserIDAndPostIDInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *DeleteFavoriteByUserIDAndPostIDInteractor) Execute(ctx context.Context, post_id int64) (bool, error) {
	user_id, err := authz.CallerID(ctx)
	if err != nil {
		return false, err
	}
	deleted, err := uc.favoriteRepo.DeleteFavoriteByUserIDAndPostID(ctx, user_id, post_id)
	if err != nil {
		return false, err
	}

	return deleted, nil
}
