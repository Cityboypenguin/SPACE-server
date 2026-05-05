package favorite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateFavoriteUseCase interface {
	Execute(ctx context.Context, param model.CreateFavoriteParam) (*model.Favorite, error)
}

var _ CreateFavoriteUseCase = &CreateFavoriteInteractor{}

type CreateFavoriteInteractor struct {
	favoriteRepo repository.FavoriteRepository
	postRepo     repository.PostRepository
}

func NewCreateFavoriteUseCase(favoriteRepo repository.FavoriteRepository, postRepo repository.PostRepository) CreateFavoriteUseCase {
	return &CreateFavoriteInteractor{
		favoriteRepo: favoriteRepo,
		postRepo:     postRepo,
	}
}

func (uc *CreateFavoriteInteractor) Execute(ctx context.Context, param model.CreateFavoriteParam) (*model.Favorite, error) {
	post, err := uc.postRepo.GetPostByID(ctx, param.PostID)
	if err != nil {
		return nil, err
	}
	if post.UserID == param.UserID {
		return nil, fmt.Errorf("ユーザーは自分の投稿をお気に入りにできません")
	}

	exist, err := uc.favoriteRepo.GetFavoriteByUserIDAndPostID(ctx, param.UserID, param.PostID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if exist != nil {
		return nil, fmt.Errorf("すでにお気に入りに登録されています")
	}

	favorite := model.CreateFavorite(param)

	id, err := uc.favoriteRepo.CreateFavorite(ctx, favorite)
	if err != nil {
		return nil, err
	}

	favorite.ID = id

	return favorite, nil
}
