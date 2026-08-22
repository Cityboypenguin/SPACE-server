package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListPollOptionResultsUseCase interface {
	Execute(ctx context.Context, pollID int64) ([]*repository.PollOptionResult, error)
}

var _ ListPollOptionResultsUseCase = &ListPollOptionResultsInteractor{}

type ListPollOptionResultsInteractor struct {
	pollRepo repository.PollRepository
}

func NewListPollOptionResultsUseCase(pollRepo repository.PollRepository) ListPollOptionResultsUseCase {
	return &ListPollOptionResultsInteractor{pollRepo: pollRepo}
}

func (uc *ListPollOptionResultsInteractor) Execute(ctx context.Context, pollID int64) ([]*repository.PollOptionResult, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.pollRepo.ListOptionsWithResults(ctx, pollID, claims.ID)
}
