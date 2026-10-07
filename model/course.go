package model

import "time"

const (
	SemesterFirst  = "前期"
	SemesterSecond = "後期"
	// SemesterFull marks a full-year (通年) course. Such a course has a single
	// courses row for the whole academic year and is meant to appear in both
	// SemesterFirst and SemesterSecond views of that year's timetable/search,
	// rather than being duplicated per term.
	SemesterFull = "通年"
)

// Course.Source の値。どこから来た授業か（シラバス同期の照合・廃止の対象になるのは
// CourseSourceSenshu だけ）。
const (
	CourseSourceSenshu = "senshu"
	CourseSourceManual = "manual"
)

type Course struct {
	ID          int64
	RoomID      int64
	DayOfWeek   string
	Period      int
	TeacherName string
	CourseName  string
	Year        int
	Semester    string
	DedupKey    string
	// Source は取り込み元（CourseSourceSenshu / CourseSourceManual）。
	Source string
	// SourceRef は取り込み元での識別子（専修大学なら講義コード）。照合のためだけに使い、
	// アプリの中で授業を指すのは ID。manual は空。
	SourceRef string
	// SourceName は校舎サフィックスを付ける前の授業名。同期前から在る行では空（不明）。
	SourceName string
	// DiscontinuedAt はシラバスから消えた日時。nil なら現行の授業。
	DiscontinuedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsDiscontinued は授業がシラバスから消えて廃止扱いになっているか。
func (c *Course) IsDiscontinued() bool {
	return c.DiscontinuedAt != nil
}
