package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
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
	// Page は窓と「total を数えるか」。他の一覧と同じ repository.PageQuery に
	// 揃えてある（以前は Limit/Offset の2フィールドだった）。
	Page repository.PageQuery
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

func (uc *ListCoursesInteractor) Execute(ctx context.Context, param ListCoursesParam) ([]*model.Course, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.courseRepo.ListCourses(ctx, repository.ListCoursesParam{
		Year:      param.Year,
		Semester:  param.Semester,
		DayOfWeek: param.DayOfWeek,
		Keyword:   param.Keyword,
		Page:      param.Page,
	})
}
