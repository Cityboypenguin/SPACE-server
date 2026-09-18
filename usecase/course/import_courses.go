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
// a new DedupKey creates a course together with its room. This is intentionally a
// plain create-if-missing, not a field-level update, so a re-run of the importer
// never overwrites data that may have diverged locally (e.g. a room that has
// already accumulated messages).
//
// 往復は入力の件数に依らず「存在確認1本＋保存（rooms と courses で2本）」に畳んである。
// 以前は1件ごとに FindByDedupKey と SaveCourseWithRoom を呼んでおり、シラバス1年ぶん
// （数千件）の取り込みで DB 往復がそのまま数千回並んでいた。取り込みは管理者が
// 進捗を眺めている間に走るので、往復の数がそのまま待ち時間になる。
//
// 進捗の更新頻度はここでは変わらない。取り込みの進捗（reportProgress）を刻んでいるのは
// スクレイピング側（FetchCourses）で、この関数は取り終わった配列を受け取るだけ。
func (uc *ImportCoursesInteractor) Execute(ctx context.Context, inputs []ScrapedCourseInput) (*ImportCoursesResult, error) {
	result := &ImportCoursesResult{}
	if len(inputs) == 0 {
		return result, nil
	}

	keys := make([]string, 0, len(inputs))
	for _, in := range inputs {
		keys = append(keys, in.DedupKey)
	}
	existing, err := uc.courseRepo.FindExistingDedupKeys(ctx, keys)
	if err != nil {
		return nil, err
	}

	// 同じ取り込みの中に同じ DedupKey が2回出てくることがある（スクレイピング側の
	// 重複行）。以前は1件ずつ「確認して保存」していたので2件目は自然に skip されたが、
	// まとめて保存する形では自分で落とさないと unique index 違反で丸ごと失敗する。
	seen := make(map[string]bool, len(inputs))

	params := make([]repository.SaveCourseParam, 0, len(inputs))
	for _, in := range inputs {
		if existing[in.DedupKey] || seen[in.DedupKey] {
			result.Skipped++
			continue
		}
		seen[in.DedupKey] = true
		params = append(params, repository.SaveCourseParam{
			DayOfWeek:   in.DayOfWeek,
			Period:      in.Period,
			TeacherName: in.TeacherName,
			CourseName:  in.CourseName,
			Year:        in.Year,
			Semester:    in.Semester,
			DedupKey:    in.DedupKey,
		})
	}

	if len(params) == 0 {
		return result, nil
	}

	saved, err := uc.courseRepo.SaveCoursesWithRooms(ctx, params)
	if err != nil {
		return nil, err
	}
	result.Imported = len(saved)
	return result, nil
}
