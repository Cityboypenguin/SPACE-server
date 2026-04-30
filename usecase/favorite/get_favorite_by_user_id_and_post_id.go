package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetFavoriteByUserIDAndPostIDUseCase interface {
	Execute(ctx context.Context, userID, postID int64) (*model.Favorite, error)
}

var _ GetFavoriteByUserIDAndPostIDUseCase = &GetFavoriteByUserIDAndPostIDInteractor{}

type GetFavoriteByUserIDAndPostIDInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewGetFavoriteByUserIDAndPostIDUseCase(favoriteRepo repository.FavoriteRepository) GetFavoriteByUserIDAndPostIDUseCase {
	return &GetFavoriteByUserIDAndPostIDInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *GetFavoriteByUserIDAndPostIDInteractor) Execute(ctx context.Context, userID, postID int64) (*model.Favorite, error) {
	return uc.favoriteRepo.GetFavoriteByUserIDAndPostID(ctx, userID, postID)
}
