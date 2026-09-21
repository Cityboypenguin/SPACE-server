package block

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchBlockersUseCase interface {
	Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.Blocker, error)
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

func (uc *searchBlockersInteractor) Execute(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.Blocker, error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	return uc.blockRepo.SearchBlockers(ctx, userID, keyword, q)
}
