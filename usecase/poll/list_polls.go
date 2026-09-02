package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListPollsUseCase interface {
	Execute(ctx context.Context, roomID int64, limit, offset int) ([]*model.Poll, int, int, error)
}

var _ ListPollsUseCase = &ListPollsInteractor{}

type ListPollsInteractor struct {
	pollRepo repository.PollRepository
}

func NewListPollsUseCase(pollRepo repository.PollRepository) ListPollsUseCase {
	return &ListPollsInteractor{pollRepo: pollRepo}
}

func (uc *ListPollsInteractor) Execute(ctx context.Context, roomID int64, limit, offset int) ([]*model.Poll, int, int, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	items, total, err := uc.pollRepo.ListPollsByRoomID(ctx, roomID, limit, offset)
	if err != nil {
		return nil, 0, 0, err
	}
	unvotedTotal, err := uc.pollRepo.CountUnvotedPollsByRoomID(ctx, roomID, claims.ID)
	if err != nil {
		return nil, 0, 0, err
	}
	return items, total, unvotedTotal, nil
}
