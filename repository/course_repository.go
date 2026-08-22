package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// SaveCourseParam holds the fields needed to create a Course together with its Room.
type SaveCourseParam struct {
	DayOfWeek   string
	Period      int
	TeacherName string
	CourseName  string
	Year        int
	Semester    string
	DedupKey    string
}

type CourseRepository interface {
	// SaveCourseWithRoom creates a Room (type=course) and a Course in a single transaction,
	// mirroring CommunityRepository.SaveCommunityWithRoom. It does not add any room_users row:
	// course rooms are readable/writable by any authenticated student, not membership-gated.
	SaveCourseWithRoom(ctx context.Context, param SaveCourseParam) (*model.Course, error)
	FindByDedupKey(ctx context.Context, dedupKey string) (*model.Course, error)
	GetCourseByID(ctx context.Context, id int64) (*model.Course, error)
	GetCourseByRoomID(ctx context.Context, roomID int64) (*model.Course, error)
	SearchByDayPeriod(ctx context.Context, dayOfWeek string, period int, keyword string, limit, offset int) ([]*model.Course, int, error)
}
