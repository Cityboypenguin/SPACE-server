package timetable

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ReplaceTimetableUseCase interface {
	Execute(ctx context.Context, year int, semesterName string, baselineEntryIDs, courseIDs []int64) ([]*repository.TimetableEntryWithCourse, error)
}

var _ ReplaceTimetableUseCase = &ReplaceTimetableInteractor{}

type ReplaceTimetableInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewReplaceTimetableUseCase(timetableRepo repository.TimetableRepository) ReplaceTimetableUseCase {
	return &ReplaceTimetableInteractor{timetableRepo: timetableRepo}
}

// Execute replaces the caller's timetable for (year, semesterName) with exactly
// courseIDs in one atomic operation (the "編集モード" batch-commit flow), rejecting
// the whole request if baselineEntryIDs no longer matches what's on record (another
// tab/session already changed the timetable since the client's baseline was loaded).
func (uc *ReplaceTimetableInteractor) Execute(ctx context.Context, year int, semesterName string, baselineEntryIDs, courseIDs []int64) ([]*repository.TimetableEntryWithCourse, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	entries, err := uc.timetableRepo.ReplaceForSemester(ctx, claims.ID, year, semesterName, baselineEntryIDs, courseIDs)
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
