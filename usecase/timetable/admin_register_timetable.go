package timetable

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AdminRegisterTimetableUseCase interface {
	Execute(ctx context.Context, userID, courseID int64) (*model.Timetable, error)
}

var _ AdminRegisterTimetableUseCase = &AdminRegisterTimetableInteractor{}

type AdminRegisterTimetableInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewAdminRegisterTimetableUseCase(timetableRepo repository.TimetableRepository) AdminRegisterTimetableUseCase {
	return &AdminRegisterTimetableInteractor{timetableRepo: timetableRepo}
}

// Execute registers courseID into userID's timetable on behalf of an administrator,
// the admin counterpart of RegisterTimetableUseCase (which always targets the
// caller's own timetable).
func (uc *AdminRegisterTimetableInteractor) Execute(ctx context.Context, userID, courseID int64) (*model.Timetable, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.timetableRepo.Upsert(ctx, userID, courseID)
}
