package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
)

type CheckRoomWritableUseCase interface {
	// Execute returns nil if roomID may currently be posted to. Non-course rooms are
	// always writable (archival only applies to course chats, F-06). For course rooms,
	// it requires both: the course's (year, semester) matches the current semester
	// setting (not archived), and the caller has the course in their timetable (only
	// registered students may write — anyone authenticated may still read).
	Execute(ctx context.Context, roomID int64) error
}

var _ CheckRoomWritableUseCase = &CheckRoomWritableInteractor{}

type CheckRoomWritableInteractor struct {
	courseRepo    repository.CourseRepository
	settingRepo   repository.SystemSettingRepository
	timetableRepo repository.TimetableRepository
}

func NewCheckRoomWritableUseCase(courseRepo repository.CourseRepository, settingRepo repository.SystemSettingRepository, timetableRepo repository.TimetableRepository) CheckRoomWritableUseCase {
	return &CheckRoomWritableInteractor{courseRepo: courseRepo, settingRepo: settingRepo, timetableRepo: timetableRepo}
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
	// 通年 (full-year) courses span both semesters of their year, so they're never
	// archived by a semester switch within the same year - only a year change ends them.
	isCurrent := c.Year == year && (c.Semester == semesterName || c.Semester == model.SemesterFull)
	if !isCurrent {
		return apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")
	}

	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}
	if !authz.IsAdminRole(claims.Role) {
		registered, err := uc.timetableRepo.IsRegistered(ctx, claims.ID, c.ID)
		if err != nil {
			return err
		}
		if !registered {
			return apperr.Forbidden("この授業を時間割に登録していないため、書き込みできません。")
		}
	}

	return nil
}
