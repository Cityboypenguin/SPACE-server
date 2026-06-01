package block

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type BlockUserUseCase interface {
	Execute(ctx context.Context, blockerID, blockedID int64) (int64, error)
}

var _ BlockUserUseCase = &blockUserInteractor{}

type blockUserInteractor struct {
	blockRepo        repository.BlockerRepository
	favoriteUserRepo repository.FavoriteUserRepository
}

func NewCreateBlockUseCase(blockRepo repository.BlockerRepository, favoriteUserRepo repository.FavoriteUserRepository) BlockUserUseCase {
	return &blockUserInteractor{
		blockRepo:        blockRepo,
		favoriteUserRepo: favoriteUserRepo,
	}
}

func (uc *blockUserInteractor) Execute(ctx context.Context, blockerID, blockedID int64) (int64, error) {
	println("【証拠】ユースケースが実行されました！ blocker:", blockerID, " blocked:", blockedID)
	if blockerID == blockedID {
		return 0, errors.New("自分自身をブロックすることはできません")
	}

	block := &model.Blocker{
		UserID:        blockerID,
		BlockedUserID: blockedID,
	}

	id, err := uc.blockRepo.CreateBlocker(ctx, block)
	if err != nil {
		return 0, err
	}

	_, _ = uc.favoriteUserRepo.DeleteFavoriteUser(ctx, blockerID, blockedID)
	_, _ = uc.favoriteUserRepo.DeleteFavoriteUser(ctx, blockedID, blockerID)

	return id, nil
}
