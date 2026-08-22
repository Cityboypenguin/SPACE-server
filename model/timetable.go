package model

import "time"

type Timetable struct {
	ID               int64
	UserID           int64
	CourseID         int64
	IsProfileVisible bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
