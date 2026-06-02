package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetBlockersByUserIDUseCase interface {
	Execute(ctx context.Context, userID int64) ([]*model.Blocker, error)
}

var _ GetBlockersByUserIDUseCase = &getBlockersByUserIDInteractor{}

type getBlockersByUserIDInteractor struct {
	blockRepo repository.BlockerRepository
}

func NewGetBlockersByUserIDUseCase(blockRepo repository.BlockerRepository) GetBlockersByUserIDUseCase {
	return &getBlockersByUserIDInteractor{
		blockRepo: blockRepo,
	}
}

func (uc *getBlockersByUserIDInteractor) Execute(ctx context.Context, userID int64) ([]*model.Blocker, error) {
	return uc.blockRepo.GetBlockersByUserID(ctx, userID)
}
