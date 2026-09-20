package favorite

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// CountFavoritesByPostIDsUseCase は投稿ごとのいいね件数を数える。
//
// GetFavoritesByPostIDsUseCase と分けてあるのは、表示に要るのが件数だけだから。
// いいね行そのものが要る経路は今のところ無いが、口を残してあるのは
// 「誰がいいねしたか」を出す画面が将来出てきたときのため。
type CountFavoritesByPostIDsUseCase interface {
	Execute(ctx context.Context, postIDs []int64) (map[int64]int, error)
}

var _ CountFavoritesByPostIDsUseCase = &CountFavoritesByPostIDsInteractor{}

type CountFavoritesByPostIDsInteractor struct {
	favoriteRepo repository.FavoriteRepository
}

func NewCountFavoritesByPostIDsUseCase(favoriteRepo repository.FavoriteRepository) CountFavoritesByPostIDsUseCase {
	return &CountFavoritesByPostIDsInteractor{favoriteRepo: favoriteRepo}
}

func (uc *CountFavoritesByPostIDsInteractor) Execute(ctx context.Context, postIDs []int64) (map[int64]int, error) {
	return uc.favoriteRepo.CountFavoritesByPostIDs(ctx, postIDs)
}
