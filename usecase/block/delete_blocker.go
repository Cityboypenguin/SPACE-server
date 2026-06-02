package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteBlockerUseCase interface {
	Execute(ctx context.Context, blockerID, blockedID int64) (bool, error)
}

var _ DeleteBlockerUseCase = &deleteBlockerInteractor{}

type deleteBlockerInteractor struct {
	blockRepo repository.BlockerRepository
}

func NewDeleteBlockerUseCase(blockRepo repository.BlockerRepository) DeleteBlockerUseCase {
	return &deleteBlockerInteractor{
		blockRepo: blockRepo,
	}
}

func (uc *deleteBlockerInteractor) Execute(ctx context.Context, blockerID int64, blockedID int64) (bool, error) {
	return uc.blockRepo.DeleteBlocker(ctx, blockerID, blockedID)
}
