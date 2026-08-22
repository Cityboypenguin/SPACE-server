package timetable

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RemoveTimetableUseCase interface {
	Execute(ctx context.Context, id int64) (bool, error)
}

var _ RemoveTimetableUseCase = &RemoveTimetableInteractor{}

type RemoveTimetableInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewRemoveTimetableUseCase(timetableRepo repository.TimetableRepository) RemoveTimetableUseCase {
	return &RemoveTimetableInteractor{timetableRepo: timetableRepo}
}

func (uc *RemoveTimetableInteractor) Execute(ctx context.Context, id int64) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	return uc.timetableRepo.Remove(ctx, id, claims.ID)
}
