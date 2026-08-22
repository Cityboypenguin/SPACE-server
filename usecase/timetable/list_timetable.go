package timetable

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
)

type ListTimetableUseCase interface {
	Execute(ctx context.Context, year *int, semesterName *string) ([]*repository.TimetableEntryWithCourse, error)
}

var _ ListTimetableUseCase = &ListTimetableInteractor{}

type ListTimetableInteractor struct {
	timetableRepo repository.TimetableRepository
	settingRepo   repository.SystemSettingRepository
}

func NewListTimetableUseCase(timetableRepo repository.TimetableRepository, settingRepo repository.SystemSettingRepository) ListTimetableUseCase {
	return &ListTimetableInteractor{timetableRepo: timetableRepo, settingRepo: settingRepo}
}

// Execute lists the caller's timetable for the given year/semester, defaulting to
// the current semester (F-06 §6.2) when both are omitted.
func (uc *ListTimetableInteractor) Execute(ctx context.Context, year *int, semesterName *string) ([]*repository.TimetableEntryWithCourse, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	y, s := 0, ""
	if year != nil && semesterName != nil {
		y, s = *year, *semesterName
	} else {
		y, s, err = semester.Get(ctx, uc.settingRepo)
		if err != nil {
			return nil, err
		}
	}

	return uc.timetableRepo.ListByUser(ctx, claims.ID, y, s)
}
