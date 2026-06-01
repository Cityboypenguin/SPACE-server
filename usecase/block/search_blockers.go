package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchBlockersUseCase interface {
	Execute(ctx context.Context, userID int64, keyword string) ([]*model.Blocker, error)
}

var _ SearchBlockersUseCase = &searchBlockersInteractor{}

type searchBlockersInteractor struct {
	blockRepo repository.BlockerRepository
}

func NewSearchBlockersUseCase(blockRepo repository.BlockerRepository) SearchBlockersUseCase {
	return &searchBlockersInteractor{
		blockRepo: blockRepo,
	}
}

func (uc *searchBlockersInteractor) Execute(ctx context.Context, userID int64, keyword string) ([]*model.Blocker, error) {
	return uc.blockRepo.SearchBlockers(ctx, userID, keyword)
}
