package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// CountPollVotersByPollIDsUseCase は投票IDの集合の投票者数をまとめて引く
// （DataLoader 用）。単体の CountPollVotersUseCase と同じく認証必須。
type CountPollVotersByPollIDsUseCase interface {
	Execute(ctx context.Context, pollIDs []int64) (map[int64]int, error)
}

var _ CountPollVotersByPollIDsUseCase = &CountPollVotersByPollIDsInteractor{}

type CountPollVotersByPollIDsInteractor struct {
	pollRepo repository.PollRepository
}

func NewCountPollVotersByPollIDsUseCase(pollRepo repository.PollRepository) CountPollVotersByPollIDsUseCase {
	return &CountPollVotersByPollIDsInteractor{pollRepo: pollRepo}
}

// Execute returns the number of distinct voters per poll. 誰も投票していない投票は
// map に入らないが、int のゼロ値がそのまま 0 人として正しい（CountPollVotersUseCase
// と同じ値になる）。
func (uc *CountPollVotersByPollIDsInteractor) Execute(ctx context.Context, pollIDs []int64) (map[int64]int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return uc.pollRepo.CountVotersByPollIDs(ctx, pollIDs)
}
