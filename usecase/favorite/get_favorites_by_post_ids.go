package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetFavoritesByPostIDsUseCase は投稿ごとのいいねを窓ぶんだけ引く。
//
// 件数と「自分がいいねしたか」だけが要る表示は CountFavoritesByPostIDs /
// ListPostIDsFavoritedBy を使うこと。こちらは「誰がいいねしたか」を出す経路用で、
// 窓を切らないと人気の投稿1つで応答が膨らむ。
type GetFavoritesByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64, q repository.PageQuery) (map[int64][]*model.Favorite, error)
}

var _ GetFavoritesByPostIDsUseCase = &GetFavoritesByPostIDsInteractor{}

type GetFavoritesByPostIDsInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewGetFavoritesByPostIDsUseCase(favoriteRepo repository.FavoriteRepository) GetFavoritesByPostIDsUseCase {
	return &GetFavoritesByPostIDsInteractor{
		favoriteRepo: favoriteRepo,
	}
}

func (uc *GetFavoritesByPostIDsInteractor) Execute(ctx context.Context, postIDs []int64, q repository.PageQuery) (map[int64][]*model.Favorite, error) {
	return uc.favoriteRepo.GetFavoritesByPostIDs(ctx, postIDs, q)
}
