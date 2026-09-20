package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetPollByIDUseCase interface {
	Execute(ctx context.Context, id int64) (*model.Poll, error)
}

var _ GetPollByIDUseCase = &GetPollByIDInteractor{}

type GetPollByIDInteractor struct {
	pollRepo repository.PollRepository
}

func NewGetPollByIDUseCase(pollRepo repository.PollRepository) GetPollByIDUseCase {
	return &GetPollByIDInteractor{pollRepo: pollRepo}
}

func (uc *GetPollByIDInteractor) Execute(ctx context.Context, id int64) (*model.Poll, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.pollRepo.GetPollByID(ctx, id)
}
