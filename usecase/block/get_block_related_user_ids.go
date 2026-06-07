package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetBlockRelatedUserIDsUseCase returns a set of user IDs that have any block
// relation (in either direction) with the given user.
type GetBlockRelatedUserIDsUseCase interface {
	Execute(ctx context.Context, userID int64) (map[int64]bool, error)
}

type getBlockRelatedUserIDsInteractor struct {
	blockRepo repository.BlockerRepository
}

func NewGetBlockRelatedUserIDsUseCase(blockRepo repository.BlockerRepository) GetBlockRelatedUserIDsUseCase {
	return &getBlockRelatedUserIDsInteractor{blockRepo: blockRepo}
}

func (uc *getBlockRelatedUserIDsInteractor) Execute(ctx context.Context, userID int64) (map[int64]bool, error) {
	ids, err := uc.blockRepo.GetBlockedAndBlockerIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}
