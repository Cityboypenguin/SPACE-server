package timetable

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SetTimetableEntryColorUseCase interface {
	Execute(ctx context.Context, id int64, color string) (*model.Timetable, error)
}

var _ SetTimetableEntryColorUseCase = &SetTimetableEntryColorInteractor{}

type SetTimetableEntryColorInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewSetTimetableEntryColorUseCase(timetableRepo repository.TimetableRepository) SetTimetableEntryColorUseCase {
	return &SetTimetableEntryColorInteractor{timetableRepo: timetableRepo}
}

func (uc *SetTimetableEntryColorInteractor) Execute(ctx context.Context, id int64, color string) (*model.Timetable, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.timetableRepo.SetColor(ctx, id, claims.ID, color)
}
