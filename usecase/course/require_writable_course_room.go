package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
)

type RequireWritableCourseRoomUseCase interface {
	// Execute returns the Course for roomID if roomID is a course room and it is
	// currently writable (F-06). Unlike CheckRoomWritableUseCase (which allows any
	// non-course room, since messages are reused everywhere), this rejects roomID
	// outright if it is not a course room at all — questions/answers/polls only
	// exist within 授業内チャット.
	Execute(ctx context.Context, roomID int64) (*model.Course, error)
}

var _ RequireWritableCourseRoomUseCase = &RequireWritableCourseRoomInteractor{}

type RequireWritableCourseRoomInteractor struct {
	courseRepo  repository.CourseRepository
	settingRepo repository.SystemSettingRepository
}

func NewRequireWritableCourseRoomUseCase(courseRepo repository.CourseRepository, settingRepo repository.SystemSettingRepository) RequireWritableCourseRoomUseCase {
	return &RequireWritableCourseRoomInteractor{courseRepo: courseRepo, settingRepo: settingRepo}
}

func (uc *RequireWritableCourseRoomInteractor) Execute(ctx context.Context, roomID int64) (*model.Course, error) {
	c, err := uc.courseRepo.GetCourseByRoomID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, apperr.InvalidInput("この機能は授業内チャットでのみ利用できます")
	}

	year, semesterName, err := semester.Get(ctx, uc.settingRepo)
	if err != nil {
		return nil, err
	}
	if c.Year != year || c.Semester != semesterName {
		return nil, apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")
	}

	return c, nil
}
