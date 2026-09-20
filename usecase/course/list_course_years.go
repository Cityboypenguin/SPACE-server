package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListCourseYearsUseCase interface {
	Execute(ctx context.Context) ([]int, error)
}

var _ ListCourseYearsUseCase = &ListCourseYearsInteractor{}

type ListCourseYearsInteractor struct {
	courseRepo repository.CourseRepository
}

func NewListCourseYearsUseCase(courseRepo repository.CourseRepository) ListCourseYearsUseCase {
	return &ListCourseYearsInteractor{courseRepo: courseRepo}
}

// Execute lists every year that has at least one course, for the admin course-listing
// year filter (auth/admin-role check is done by the resolver, matching ListCoursesUseCase).
func (uc *ListCourseYearsInteractor) Execute(ctx context.Context) ([]int, error) {
	return uc.courseRepo.ListDistinctYears(ctx)
}
