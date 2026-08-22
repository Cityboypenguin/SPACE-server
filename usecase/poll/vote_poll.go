package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type VotePollUseCase interface {
	Execute(ctx context.Context, pollID int64, optionIDs []int64) error
}

var _ VotePollUseCase = &VotePollInteractor{}

type VotePollInteractor struct {
	pollRepo        repository.PollRepository
	requireWritable course.RequireWritableCourseRoomUseCase
}

func NewVotePollUseCase(pollRepo repository.PollRepository, requireWritable course.RequireWritableCourseRoomUseCase) VotePollUseCase {
	return &VotePollInteractor{pollRepo: pollRepo, requireWritable: requireWritable}
}

// Execute replaces the caller's vote(s) on pollID with optionIDs (I-03: 投票のやり直し
// を許容する). For single-choice polls, more than one optionID is rejected.
func (uc *VotePollInteractor) Execute(ctx context.Context, pollID int64, optionIDs []int64) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}
	if len(optionIDs) == 0 {
		return apperr.InvalidInput("選択肢を1つ以上選んでください")
	}

	p, err := uc.pollRepo.GetPollByID(ctx, pollID)
	if err != nil {
		return err
	}
	if p == nil {
		return apperr.NotFound("投票が見つかりません")
	}
	if _, err := uc.requireWritable.Execute(ctx, p.RoomID); err != nil {
		return err
	}

	optionIDs = dedupe(optionIDs)
	if !p.AllowMultipleChoice && len(optionIDs) > 1 {
		return apperr.InvalidInput("この投票は単一選択のみです")
	}

	return uc.pollRepo.ReplaceVotes(ctx, pollID, claims.ID, optionIDs)
}

func dedupe(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
