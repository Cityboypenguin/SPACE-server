package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListBlockersUseCase interface {
	Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Blocker, int, error)
}

var _ ListBlockersUseCase = &listBlockersInteractor{}

type listBlockersInteractor struct {
	blockRepo repository.BlockerRepository
}

func NewListBlockersUseCase(blockRepo repository.BlockerRepository) ListBlockersUseCase {
	return &listBlockersInteractor{
		blockRepo: blockRepo,
	}
}

func (uc *listBlockersInteractor) Execute(ctx context.Context, userID int64, limit, offset int) ([]*model.Blocker, int, error) {
	return uc.blockRepo.ListBlockers(ctx, userID, limit, offset)
}
