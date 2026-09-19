package poll

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type VotePollUseCase interface {
	Execute(ctx context.Context, pollID int64, optionIDs []int64) (*model.Poll, error)
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
// を許容する). An empty optionIDs cancels the caller's vote entirely. For
// single-choice polls, more than one optionID is rejected.
func (uc *VotePollInteractor) Execute(ctx context.Context, pollID int64, optionIDs []int64) (*model.Poll, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	p, err := uc.pollRepo.GetPollByID(ctx, pollID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperr.NotFound("投票が見つかりません")
	}
	if p.Deadline != nil && !time.Now().Before(*p.Deadline) {
		return nil, apperr.InvalidInput("回答期限を過ぎているため投票できません")
	}
	if _, err := uc.requireWritable.Execute(ctx, p.RoomID); err != nil {
		return nil, err
	}

	optionIDs = dedupe(optionIDs)
	if !p.AllowMultipleChoice && len(optionIDs) > 1 {
		return nil, apperr.InvalidInput("この投票は単一選択のみです")
	}

	if err := uc.pollRepo.ReplaceVotes(ctx, pollID, claims.ID, optionIDs); err != nil {
		return nil, err
	}
	updated, err := uc.pollRepo.GetPollByID(ctx, pollID)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, errors.New("poll not found after vote update")
	}
	return updated, nil
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
