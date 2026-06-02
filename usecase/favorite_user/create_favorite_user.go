package favoriteuser

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateFavoriteUserUseCase interface {
	Execute(ctx context.Context, userID, favoriteUserID int64) (int64, error)
}

var _ CreateFavoriteUserUseCase = &createFavoriteUserInteractor{}

type createFavoriteUserInteractor struct {
	favoriteUserRepo repository.FavoriteUserRepository
	blockRepo        repository.BlockerRepository
}

func NewCreateFavoriteUserUseCase(favoriteUserRepo repository.FavoriteUserRepository, blockRepo repository.BlockerRepository) CreateFavoriteUserUseCase {
	return &createFavoriteUserInteractor{
		favoriteUserRepo: favoriteUserRepo,
		blockRepo:        blockRepo,
	}
}

func (uc *createFavoriteUserInteractor) Execute(ctx context.Context, userID, favoriteUserID int64) (int64, error) {
	exists, err := uc.blockRepo.ExistsBlockRelation(ctx, userID, favoriteUserID)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, errors.New("ブロック関係が存在するため、お気に入り登録できません")
	}
	if userID == favoriteUserID {
		return 0, errors.New("自分自身をお気に入りユーザーにすることはできません")
	}

	favoriteUser := &model.FavoriteUser{
		UserID:         userID,
		FavoriteUserID: favoriteUserID,
	}

	return uc.favoriteUserRepo.CreateFavoriteUser(ctx, favoriteUser)
}
