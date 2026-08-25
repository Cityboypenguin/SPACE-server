package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListCoursesParam mirrors repository.ListCoursesParam; kept as a separate type so the
// resolver layer doesn't depend on the repository package directly.
type ListCoursesParam struct {
	Year      *int
	Semester  *string
	DayOfWeek *string
	Keyword   string
	Limit     int
	Offset    int
}

type ListCoursesUseCase interface {
	Execute(ctx context.Context, param ListCoursesParam) ([]*model.Course, int, error)
}

var _ ListCoursesUseCase = &ListCoursesInteractor{}

type ListCoursesInteractor struct {
	courseRepo repository.CourseRepository
}

func NewListCoursesUseCase(courseRepo repository.CourseRepository) ListCoursesUseCase {
	return &ListCoursesInteractor{courseRepo: courseRepo}
}

// Execute lists courses for the admin course-management screen (auth/admin-role check
// is done by the resolver, matching AdminTriggerCourseImport's convention).
func (uc *ListCoursesInteractor) Execute(ctx context.Context, param ListCoursesParam) ([]*model.Course, int, error) {
	return uc.courseRepo.ListCourses(ctx, repository.ListCoursesParam{
		Year:      param.Year,
		Semester:  param.Semester,
		DayOfWeek: param.DayOfWeek,
		Keyword:   param.Keyword,
		Limit:     param.Limit,
		Offset:    param.Offset,
	})
}
