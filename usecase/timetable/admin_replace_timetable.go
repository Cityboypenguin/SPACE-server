package timetable

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AdminReplaceTimetableUseCase interface {
	Execute(ctx context.Context, userID int64, year int, semesterName string, baselineEntryIDs, courseIDs []int64) ([]*repository.TimetableEntryWithCourse, error)
}

var _ AdminReplaceTimetableUseCase = &AdminReplaceTimetableInteractor{}

type AdminReplaceTimetableInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewAdminReplaceTimetableUseCase(timetableRepo repository.TimetableRepository) AdminReplaceTimetableUseCase {
	return &AdminReplaceTimetableInteractor{timetableRepo: timetableRepo}
}

// Execute replaces userID's timetable for (year, semesterName) on behalf of an
// administrator, the admin counterpart of ReplaceTimetableUseCase.
func (uc *AdminReplaceTimetableInteractor) Execute(ctx context.Context, userID int64, year int, semesterName string, baselineEntryIDs, courseIDs []int64) ([]*repository.TimetableEntryWithCourse, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	entries, err := uc.timetableRepo.ReplaceForSemester(ctx, userID, year, semesterName, baselineEntryIDs, courseIDs)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrTimetableConflict):
			return nil, apperr.Conflict("他の操作で時間割が変更されています。最新の状態を確認してください。")
		case errors.Is(err, repository.ErrTimetableSlotConflict):
			return nil, apperr.InvalidInput("同じ曜日・時限に複数の授業を登録することはできません。")
		default:
			return nil, err
		}
	}
	return entries, nil
}
