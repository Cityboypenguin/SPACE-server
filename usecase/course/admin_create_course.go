package course

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/google/uuid"
)

// validDaysOfWeek and validSemesters mirror the values the scraper itself produces
// (see infra/scraper/su_syllabus.go's periodCellRe and model.Semester* constants) -
// a manually created course must fit the same value space or it silently fails to
// render in the timetable grid / current-semester writability check.
var validDaysOfWeek = map[string]bool{"月": true, "火": true, "水": true, "木": true, "金": true, "土": true}

const (
	minPeriod = 1
	maxPeriod = 7
)

func isValidSemester(s string) bool {
	return s == model.SemesterFirst || s == model.SemesterSecond || s == model.SemesterFull
}

// AdminCreateCourseParam holds the fields an admin supplies to manually add a
// course the scraper hasn't (yet, or ever will) picked up.
type AdminCreateCourseParam struct {
	DayOfWeek   string
	Period      int
	TeacherName string
	CourseName  string
	Year        int
	Semester    string
}

type AdminCreateCourseUseCase interface {
	Execute(ctx context.Context, param AdminCreateCourseParam) (*model.Course, error)
}

var _ AdminCreateCourseUseCase = &AdminCreateCourseInteractor{}

type AdminCreateCourseInteractor struct {
	courseRepo repository.CourseRepository
}

func NewAdminCreateCourseUseCase(courseRepo repository.CourseRepository) AdminCreateCourseUseCase {
	return &AdminCreateCourseInteractor{courseRepo: courseRepo}
}

// Execute creates a course (and its chat room, via SaveCourseWithRoom) with a
// "manual:" dedup_key so a later scrape of the same year can never match it by
// DedupKey and treat it as already-imported for a different course, nor have this
// row mistaken for one of the scraper's own "senshu:..." keys.
//
// Auth/admin-role check is done by the resolver (matching ListCoursesUseCase's
// convention in this package).
func (uc *AdminCreateCourseInteractor) Execute(ctx context.Context, param AdminCreateCourseParam) (*model.Course, error) {
	if !validDaysOfWeek[param.DayOfWeek] {
		return nil, apperr.InvalidInput("曜日が不正です")
	}
	if param.Period < minPeriod || param.Period > maxPeriod {
		return nil, apperr.InvalidInput(fmt.Sprintf("時限は%d〜%dの範囲で指定してください", minPeriod, maxPeriod))
	}
	if !isValidSemester(param.Semester) {
		return nil, apperr.InvalidInput("学期が不正です")
	}
	if param.TeacherName == "" || param.CourseName == "" {
		return nil, apperr.InvalidInput("教員名・授業名は必須です")
	}

	return uc.courseRepo.SaveCourseWithRoom(ctx, repository.SaveCourseParam{
		DayOfWeek:   param.DayOfWeek,
		Period:      param.Period,
		TeacherName: param.TeacherName,
		CourseName:  param.CourseName,
		Year:        param.Year,
		Semester:    param.Semester,
		DedupKey:    "manual:" + uuid.New().String(),
	})
}
