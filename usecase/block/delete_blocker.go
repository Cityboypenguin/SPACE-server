package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteBlockerUseCase interface {
	Execute(ctx context.Context, blockedID int64) (bool, error)
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

func (uc *deleteBlockerInteractor) Execute(ctx context.Context, blockedID int64) (bool, error) {
	blockerID, err := authz.CallerID(ctx)
	if err != nil {
		return false, err
	}
	return uc.blockRepo.DeleteBlocker(ctx, blockerID, blockedID)
}
