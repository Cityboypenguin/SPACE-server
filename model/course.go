package model

import "time"

const (
	SemesterFirst  = "前期"
	SemesterSecond = "後期"
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
