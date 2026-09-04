package timetable

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AdminRemoveTimetableUseCase interface {
	Execute(ctx context.Context, id, userID int64) (bool, error)
}

var _ AdminRemoveTimetableUseCase = &AdminRemoveTimetableInteractor{}

type AdminRemoveTimetableInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewAdminRemoveTimetableUseCase(timetableRepo repository.TimetableRepository) AdminRemoveTimetableUseCase {
	return &AdminRemoveTimetableInteractor{timetableRepo: timetableRepo}
}

// Execute removes timetable entry id belonging to userID on behalf of an
// administrator, the admin counterpart of RemoveTimetableUseCase.
func (uc *AdminRemoveTimetableInteractor) Execute(ctx context.Context, id, userID int64) (bool, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return false, err
	}
	return uc.timetableRepo.Remove(ctx, id, userID)
}
