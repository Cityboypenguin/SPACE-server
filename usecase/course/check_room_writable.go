package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
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

// 判定の中身は courseRoomWritePolicy が持つ。ここは「授業ルームでなければ通す」
// という、このユースケース固有の振る舞いだけを足している。
type CheckRoomWritableInteractor struct {
	policy courseRoomWritePolicy
}

func NewCheckRoomWritableUseCase(courseRepo repository.CourseRepository, settingRepo repository.SystemSettingRepository, timetableRepo repository.TimetableRepository) CheckRoomWritableUseCase {
	return &CheckRoomWritableInteractor{
		policy: courseRoomWritePolicy{courseRepo: courseRepo, settingRepo: settingRepo, timetableRepo: timetableRepo},
	}
}

func (uc *CheckRoomWritableInteractor) Execute(ctx context.Context, roomID int64) error {
	// ensureWritable は授業ルームでないとき (nil, nil) を返す。メッセージは
	// コミュニティ・DM でも使うため、その場合はアーカイブ判定の対象外として通す。
	_, err := uc.policy.ensureWritable(ctx, roomID)
	return err
}
