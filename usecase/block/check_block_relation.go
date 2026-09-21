package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CheckBlockRelationUseCase interface {
	Execute(ctx context.Context, targetUserID int64) (bool, error)
}

var _ CheckBlockRelationUseCase = &checkBlockRelationInteractor{}

type checkBlockRelationInteractor struct {
	blockRepo repository.BlockerRepository
}

func NewCheckBlockRelationUseCase(blockRepo repository.BlockerRepository) CheckBlockRelationUseCase {
	return &checkBlockRelationInteractor{
		blockRepo: blockRepo,
	}
}

func (uc *checkBlockRelationInteractor) Execute(ctx context.Context, targetUserID int64) (bool, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return false, err
	}
	return uc.blockRepo.ExistsBlockRelation(ctx, userID, targetUserID)
}
