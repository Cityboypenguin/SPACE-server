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

// ListCoursesParam holds the optional filters for ListCourses (admin course listing).
// A nil Year/Semester/DayOfWeek means "no filter on this field"; an empty Keyword means
// no course-name/teacher-name filter.
type ListCoursesParam struct {
	Year      *int
	Semester  *string
	DayOfWeek *string
	Keyword   string
	Limit     int
	Offset    int
}

type CourseRepository interface {
	// SaveCourseWithRoom creates a Room (type=course) and a Course in a single transaction,
	// mirroring CommunityRepository.SaveCommunityWithRoom. It does not add any room_users row:
	// course rooms are readable/writable by any authenticated student, not membership-gated.
	SaveCourseWithRoom(ctx context.Context, param SaveCourseParam) (*model.Course, error)
	FindByDedupKey(ctx context.Context, dedupKey string) (*model.Course, error)
	GetCourseByID(ctx context.Context, id int64) (*model.Course, error)
	GetCourseByRoomID(ctx context.Context, roomID int64) (*model.Course, error)
	SearchByDayPeriod(ctx context.Context, dayOfWeek string, period int, keyword string, year int, semester string, limit, offset int) ([]*model.Course, int, error)
	ListCourses(ctx context.Context, param ListCoursesParam) ([]*model.Course, int, error)
	// ListDistinctYears returns every year present in courses, newest first, so the
	// admin course-management screen can offer a year picker backed by actual data
	// instead of an arbitrary free-typed number.
	ListDistinctYears(ctx context.Context) ([]int, error)
}
