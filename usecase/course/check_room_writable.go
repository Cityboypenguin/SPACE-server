package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
)

type CheckRoomWritableUseCase interface {
	// Execute returns nil if roomID may currently be posted to. Non-course rooms are
	// always writable (archival only applies to course chats, F-06). For course rooms,
	// it compares the course's (year, semester) against the current semester setting;
	// a mismatch means the room is archived (read-only).
	Execute(ctx context.Context, roomID int64) error
}

var _ CheckRoomWritableUseCase = &CheckRoomWritableInteractor{}

type CheckRoomWritableInteractor struct {
	courseRepo  repository.CourseRepository
	settingRepo repository.SystemSettingRepository
}

func NewCheckRoomWritableUseCase(courseRepo repository.CourseRepository, settingRepo repository.SystemSettingRepository) CheckRoomWritableUseCase {
	return &CheckRoomWritableInteractor{courseRepo: courseRepo, settingRepo: settingRepo}
}

func (uc *CheckRoomWritableInteractor) Execute(ctx context.Context, roomID int64) error {
	c, err := uc.courseRepo.GetCourseByRoomID(ctx, roomID)
	if err != nil {
		return err
	}
	if c == nil {
		// Not a course room; archival does not apply.
		return nil
	}

	year, semesterName, err := semester.Get(ctx, uc.settingRepo)
	if err != nil {
		return err
	}
	if c.Year != year || c.Semester != semesterName {
		return apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")
	}
	return nil
}
