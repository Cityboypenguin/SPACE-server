package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeletePollUseCase interface {
	Execute(ctx context.Context, pollID int64) (*model.Poll, error)
}

var _ DeletePollUseCase = &DeletePollInteractor{}

type DeletePollInteractor struct {
	pollRepo repository.PollRepository
}

func NewDeletePollUseCase(pollRepo repository.PollRepository) DeletePollUseCase {
	return &DeletePollInteractor{pollRepo: pollRepo}
}

// Execute deletes a poll (and its options/votes), allowed for the poll's own
// author or an administrator moderating 授業内チャット. It returns the poll as
// it was just before deletion, so the caller can notify subscribers.
func (uc *DeletePollInteractor) Execute(ctx context.Context, pollID int64) (*model.Poll, error) {
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
	if p.AuthorUserID != claims.ID && !authz.IsAdminRole(claims.Role) {
		return nil, apperr.Forbidden("この投票を削除する権限がありません")
	}

	if _, err := uc.pollRepo.DeletePoll(ctx, pollID); err != nil {
		return nil, err
	}
	return p, nil
}
