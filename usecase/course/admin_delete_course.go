package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type AdminDeleteCourseUseCase interface {
	Execute(ctx context.Context, courseID int64) (bool, error)
}

var _ AdminDeleteCourseUseCase = &AdminDeleteCourseInteractor{}

type AdminDeleteCourseInteractor struct {
	courseRepo repository.CourseRepository
	roomRepo   repository.RoomRepository
}

func NewAdminDeleteCourseUseCase(courseRepo repository.CourseRepository, roomRepo repository.RoomRepository) AdminDeleteCourseUseCase {
	return &AdminDeleteCourseInteractor{courseRepo: courseRepo, roomRepo: roomRepo}
}

// Execute deletes courseID by deleting its chat room instead of the courses row
// directly: courses.room_id -> rooms(id) is ON DELETE CASCADE (see
// db/migrations/045_create_courses.up.sql), and from there timetables.course_id ->
// courses(id) is itself ON DELETE CASCADE too, so deleting the room transitively
// wipes the course, every user's timetable registration for it, and the room's
// messages/questions/polls/anonymous identities in one atomic DB-level cascade.
// There is no separate "delete the courses row but keep the room" operation to
// reach for - that would leave an orphaned, permanently-empty course room behind.
//
// Auth/admin-role check is done by the resolver (matching ListCoursesUseCase's
// convention in this package). The resolver is also expected to have surfaced
// Course.registeredCount to the admin beforehand so they know how many students'
// registrations this will take with it - this use case does not re-check that or
// require confirmation itself.
func (uc *AdminDeleteCourseInteractor) Execute(ctx context.Context, courseID int64) (bool, error) {
	c, err := uc.courseRepo.GetCourseByID(ctx, courseID)
	if err != nil {
		return false, err
	}
	if c == nil {
		return false, apperr.NotFound("授業が見つかりません")
	}
	return uc.roomRepo.DeleteRoom(ctx, c.RoomID)
}
