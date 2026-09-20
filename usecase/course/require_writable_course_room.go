package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RequireWritableCourseRoomUseCase interface {
	// Execute returns the Course for roomID if roomID is a course room and it is
	// currently writable (F-06): not archived, and the caller has the course in
	// their timetable (only registered students may post questions/answers/polls).
	// Unlike CheckRoomWritableUseCase (which allows any non-course room, since
	// messages are reused everywhere), this rejects roomID outright if it is not a
	// course room at all — questions/answers/polls only exist within 授業内チャット.
	Execute(ctx context.Context, roomID int64) (*model.Course, error)
}

var _ RequireWritableCourseRoomUseCase = &RequireWritableCourseRoomInteractor{}

// 判定の中身は courseRoomWritePolicy が持つ。ここは「授業ルームでなければ拒否」
// という、このユースケース固有の振る舞いだけを足している。
type RequireWritableCourseRoomInteractor struct {
	policy courseRoomWritePolicy
}

func NewRequireWritableCourseRoomUseCase(courseRepo repository.CourseRepository, settingRepo repository.SystemSettingRepository, timetableRepo repository.TimetableRepository) RequireWritableCourseRoomUseCase {
	return &RequireWritableCourseRoomInteractor{
		policy: courseRoomWritePolicy{courseRepo: courseRepo, settingRepo: settingRepo, timetableRepo: timetableRepo},
	}
}

func (uc *RequireWritableCourseRoomInteractor) Execute(ctx context.Context, roomID int64) (*model.Course, error) {
	c, err := uc.policy.ensureWritable(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, apperr.InvalidInput("この機能は授業内チャットでのみ利用できます")
	}
	return c, nil
}
