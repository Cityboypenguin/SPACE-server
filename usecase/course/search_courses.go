package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
)

type SearchCoursesUseCase interface {
	Execute(ctx context.Context, dayOfWeek string, period int, keyword string, limit, offset int) ([]*model.Course, int, error)
}

var _ SearchCoursesUseCase = &SearchCoursesInteractor{}

type SearchCoursesInteractor struct {
	courseRepo  repository.CourseRepository
	settingRepo repository.SystemSettingRepository
}

func NewSearchCoursesUseCase(courseRepo repository.CourseRepository, settingRepo repository.SystemSettingRepository) SearchCoursesUseCase {
	return &SearchCoursesInteractor{courseRepo: courseRepo, settingRepo: settingRepo}
}

// Execute lists courses offered in a given day/period slot (F-01: students pick the
// slot first), optionally narrowed further by a course-name/teacher-name keyword.
// Results are always scoped to the current semester: the same day/period slot can be
// occupied by unrelated courses across 前期/後期 (or past years), and mixing them
// together in one result list would be confusing and would let students register
// for a course that isn't actually offered this term.
func (uc *SearchCoursesInteractor) Execute(ctx context.Context, dayOfWeek string, period int, keyword string, limit, offset int) ([]*model.Course, int, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return nil, 0, err
	}
	year, semesterName, err := semester.Get(ctx, uc.settingRepo)
	if err != nil {
		return nil, 0, err
	}
	return uc.courseRepo.SearchByDayPeriod(ctx, dayOfWeek, period, keyword, year, semesterName, limit, offset)
}
