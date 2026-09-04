package timetable

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AdminSetTimetableEntryColorUseCase interface {
	Execute(ctx context.Context, id, userID int64, color string) (*model.Timetable, error)
}

var _ AdminSetTimetableEntryColorUseCase = &AdminSetTimetableEntryColorInteractor{}

type AdminSetTimetableEntryColorInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewAdminSetTimetableEntryColorUseCase(timetableRepo repository.TimetableRepository) AdminSetTimetableEntryColorUseCase {
	return &AdminSetTimetableEntryColorInteractor{timetableRepo: timetableRepo}
}

// Execute updates entry id's color on behalf of an administrator, the admin
// counterpart of SetTimetableEntryColorUseCase.
func (uc *AdminSetTimetableEntryColorInteractor) Execute(ctx context.Context, id, userID int64, color string) (*model.Timetable, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.timetableRepo.SetColor(ctx, id, userID, color)
}
