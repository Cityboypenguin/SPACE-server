package block

import (
	"context"

	"errors"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type BlockUserUseCase interface {
	Execute(ctx context.Context, blockedID int64) (int64, error)
}

var _ BlockUserUseCase = &blockUserInteractor{}

type blockUserInteractor struct {
	blockRepo        repository.BlockerRepository
	favoriteUserRepo repository.FavoriteUserRepository
	txManager        repository.TxManager
}

func NewCreateBlockUseCase(blockRepo repository.BlockerRepository, favoriteUserRepo repository.FavoriteUserRepository, txManager repository.TxManager) BlockUserUseCase {
	return &blockUserInteractor{
		blockRepo:        blockRepo,
		favoriteUserRepo: favoriteUserRepo,
		txManager:        txManager,
	}
}

func (uc *blockUserInteractor) Execute(ctx context.Context, blockedID int64) (int64, error) {
	blockerID, err := authz.CallerID(ctx)
	if err != nil {
		return 0, err
	}
	if blockerID == blockedID {
		return 0, errors.New("自分自身をブロックすることはできません")
	}

	var blockID int64
	err = uc.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		var err error
		block := &model.Blocker{
			UserID:        blockerID,
			BlockedUserID: blockedID,
		}
		blockID, err = uc.blockRepo.CreateBlocker(txCtx, block)
		if err != nil {
			// 既にブロック済みなら成功として扱う（ブロックは冪等）。
			// ドライバのエラー番号ではなく、リポジトリ共通の番兵で判定する。
			if errors.Is(err, repository.ErrDuplicateKey) {
				return nil
			}
			return err
		}

		_, err = uc.favoriteUserRepo.DeleteFavoriteUser(txCtx, blockerID, blockedID)
		if err != nil {
			return err
		}

		_, err = uc.favoriteUserRepo.DeleteFavoriteUser(txCtx, blockedID, blockerID)
		if err != nil {
			return err
		}

		return nil
	})

	return blockID, err
}
