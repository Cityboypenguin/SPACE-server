package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListPostIDsFavoritedByUseCase は postIDs のうち userID がいいねしたものを返す。
//
// 「自分がいいねしたか」の表示に使う。いいね行を全部取って自分を探すのと違い、
// 戻る行数は投稿の人気に左右されない。
type ListPostIDsFavoritedByUseCase interface {
	Execute(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error)
}

var _ ListPostIDsFavoritedByUseCase = &ListPostIDsFavoritedByInteractor{}

type ListPostIDsFavoritedByInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewListPostIDsFavoritedByUseCase(favoriteRepo repository.FavoriteRepository) ListPostIDsFavoritedByUseCase {
	return &ListPostIDsFavoritedByInteractor{favoriteRepo: favoriteRepo}
}

func (uc *ListPostIDsFavoritedByInteractor) Execute(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error) {
	return uc.favoriteRepo.ListPostIDsFavoritedBy(ctx, userID, postIDs)
}
