package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteFavoriteByUserIDAndPostIDUseCase interface {
	Execute(ctx context.Context, user_id int64, post_id int64) (bool, error)
}

var _ DeleteFavoriteByUserIDAndPostIDUseCase = &DeleteFavoriteByUserIDAndPostIDInteractor{}

type DeleteFavoriteByUserIDAndPostIDInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewDeleteFavoriteByUserIDAndPostIDUseCase(favoriteRepo repository.FavoriteRepository, postRepo repository.PostRepository) DeleteFavoriteByUserIDAndPostIDUseCase {
	return &DeleteFavoriteByUserIDAndPostIDInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *DeleteFavoriteByUserIDAndPostIDInteractor) Execute(ctx context.Context, user_id int64, post_id int64) (bool, error) {
	deleted, err := uc.favoriteRepo.DeleteFavoriteByUserIDAndPostID(ctx, user_id, post_id)
	if err != nil {
		return false, err
	}

	return deleted, nil
}
