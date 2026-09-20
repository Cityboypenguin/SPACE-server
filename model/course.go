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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
