package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListBlockersUseCase interface {
	Execute(ctx context.Context, q repository.PageQuery) ([]*model.Blocker, int, error)
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

func (uc *listBlockersInteractor) Execute(ctx context.Context, q repository.PageQuery) ([]*model.Blocker, int, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return uc.blockRepo.ListBlockers(ctx, userID, q)
}
