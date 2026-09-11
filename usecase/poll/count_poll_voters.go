package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CountPollVotersUseCase interface {
	Execute(ctx context.Context, pollID int64) (int, error)
}

var _ CountPollVotersUseCase = &CountPollVotersInteractor{}

type CountPollVotersInteractor struct {
	pollRepo repository.PollRepository
}

func NewCountPollVotersUseCase(pollRepo repository.PollRepository) CountPollVotersUseCase {
	return &CountPollVotersInteractor{pollRepo: pollRepo}
}

// Execute returns the number of distinct users who voted on pollID. Summing
// per-option vote counts over-counts multiple-choice polls, so the 「N人が回答済み」
// display must use this instead.
func (uc *CountPollVotersInteractor) Execute(ctx context.Context, pollID int64) (int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return 0, err
	}
	return uc.pollRepo.CountVoters(ctx, pollID)
}
