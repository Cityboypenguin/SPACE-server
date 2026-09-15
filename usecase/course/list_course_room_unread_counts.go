package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
)

type ListCourseRoomUnreadCountsUseCase interface {
	Execute(ctx context.Context) ([]*repository.CourseRoomUnread, error)
}

var _ ListCourseRoomUnreadCountsUseCase = &ListCourseRoomUnreadCountsInteractor{}

type ListCourseRoomUnreadCountsInteractor struct {
	messageRepo repository.MessageRepository
	settingRepo repository.SystemSettingRepository
}

func NewListCourseRoomUnreadCountsUseCase(messageRepo repository.MessageRepository, settingRepo repository.SystemSettingRepository) ListCourseRoomUnreadCountsUseCase {
	return &ListCourseRoomUnreadCountsInteractor{messageRepo: messageRepo, settingRepo: settingRepo}
}

// Execute returns the caller's unread count for each course chat in their timetable
// for the current semester, in one query (a badge per 授業 would otherwise need one
// query per room). Courses from past semesters are not included: they are read-only.
func (uc *ListCourseRoomUnreadCountsInteractor) Execute(ctx context.Context) ([]*repository.CourseRoomUnread, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	year, semesterName, err := semester.Get(ctx, uc.settingRepo)
	if err != nil {
		return nil, err
	}

	return uc.messageRepo.CountUnreadByCourseRooms(ctx, claims.ID, year, semesterName)
}
