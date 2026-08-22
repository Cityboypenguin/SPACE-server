package semester

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateCurrentSemesterUseCase interface {
	Execute(ctx context.Context, year int, semesterName string) error
}

var _ UpdateCurrentSemesterUseCase = &UpdateCurrentSemesterInteractor{}

type UpdateCurrentSemesterInteractor struct {
	settingRepo repository.SystemSettingRepository
}

func NewUpdateCurrentSemesterUseCase(settingRepo repository.SystemSettingRepository) UpdateCurrentSemesterUseCase {
	return &UpdateCurrentSemesterInteractor{settingRepo: settingRepo}
}

// Execute updates the current semester. Only administrators may change it, since an
// incorrect value affects every student's default timetable view and every course
// room's writability (see usecase/course.CheckRoomWritableUseCase in Phase 3).
func (uc *UpdateCurrentSemesterInteractor) Execute(ctx context.Context, year int, semesterName string) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return Set(ctx, uc.settingRepo, year, semesterName, time.Now().Unix())
}
