package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchCoursesUseCase interface {
	Execute(ctx context.Context, dayOfWeek string, period int, keyword string, limit, offset int) ([]*model.Course, int, error)
}

var _ SearchCoursesUseCase = &SearchCoursesInteractor{}

type SearchCoursesInteractor struct {
	courseRepo repository.CourseRepository
}

func NewSearchCoursesUseCase(courseRepo repository.CourseRepository) SearchCoursesUseCase {
	return &SearchCoursesInteractor{courseRepo: courseRepo}
}

// Execute lists courses offered in a given day/period slot (F-01: students pick the
// slot first), optionally narrowed further by a course-name/teacher-name keyword.
func (uc *SearchCoursesInteractor) Execute(ctx context.Context, dayOfWeek string, period int, keyword string, limit, offset int) ([]*model.Course, int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, 0, err
	}
	return uc.courseRepo.SearchByDayPeriod(ctx, dayOfWeek, period, keyword, limit, offset)
}
