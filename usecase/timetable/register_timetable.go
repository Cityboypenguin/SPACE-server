package timetable

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RegisterTimetableUseCase interface {
	Execute(ctx context.Context, courseID int64) (*model.Timetable, error)
}

var _ RegisterTimetableUseCase = &RegisterTimetableInteractor{}

type RegisterTimetableInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewRegisterTimetableUseCase(timetableRepo repository.TimetableRepository) RegisterTimetableUseCase {
	return &RegisterTimetableInteractor{timetableRepo: timetableRepo}
}

// Execute registers courseID into the caller's timetable. There is no enrollment
// check (F-02: self-reported, no enforcement); registering into an already-filled
// slot replaces the existing entry there.
func (uc *RegisterTimetableInteractor) Execute(ctx context.Context, courseID int64) (*model.Timetable, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := uc.timetableRepo.Upsert(ctx, claims.ID, courseID)
	if errors.Is(err, repository.ErrCourseDiscontinued) {
		return nil, apperr.InvalidInput("廃止された授業（シラバスに掲載されなくなった授業）は新しく登録できません。")
	}
	return entry, err
}
