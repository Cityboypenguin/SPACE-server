package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ScrapedCourseInput is the shape any course data source (scraper, CSV import, manual
// admin entry, ...) must produce. Keeping it decoupled from the scraping mechanism
// means the parser can be swapped without touching the import logic below.
type ScrapedCourseInput struct {
	DayOfWeek   string
	Period      int
	TeacherName string
	CourseName  string
	Year        int
	Semester    string
	DedupKey    string
}

// ImportCoursesResult reports how many of the submitted inputs were newly created
// versus already present (matched by DedupKey), so a batch run can be summarized.
type ImportCoursesResult struct {
	Imported int
	Skipped  int
}

type ImportCoursesUseCase interface {
	Execute(ctx context.Context, inputs []ScrapedCourseInput) (*ImportCoursesResult, error)
}

var _ ImportCoursesUseCase = &ImportCoursesInteractor{}

type ImportCoursesInteractor struct {
	courseRepo repository.CourseRepository
}

func NewImportCoursesUseCase(courseRepo repository.CourseRepository) ImportCoursesUseCase {
	return &ImportCoursesInteractor{courseRepo: courseRepo}
}

// Execute upserts by DedupKey: an existing course (and its room) is left untouched,
// a new DedupKey creates a course together with its room via SaveCourseWithRoom.
// This is intentionally a plain create-if-missing, not a field-level update, so a
// re-run of the importer never overwrites data that may have diverged locally (e.g.
// a room that has already accumulated messages).
func (uc *ImportCoursesInteractor) Execute(ctx context.Context, inputs []ScrapedCourseInput) (*ImportCoursesResult, error) {
	result := &ImportCoursesResult{}
	for _, in := range inputs {
		existing, err := uc.courseRepo.FindByDedupKey(ctx, in.DedupKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			result.Skipped++
			continue
		}

		if _, err := uc.courseRepo.SaveCourseWithRoom(ctx, repository.SaveCourseParam{
			DayOfWeek:   in.DayOfWeek,
			Period:      in.Period,
			TeacherName: in.TeacherName,
			CourseName:  in.CourseName,
			Year:        in.Year,
			Semester:    in.Semester,
			DedupKey:    in.DedupKey,
		}); err != nil {
			return nil, err
		}
		result.Imported++
	}
	return result, nil
}
