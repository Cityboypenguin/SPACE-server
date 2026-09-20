package model

import "time"

// TimetableEntryColorDefault is the color assigned to a timetable entry on
// registration, before the user has picked one from the palette. It must stay in
// sync with the DB column's DEFAULT and the GraphQL TimetableEntryColor enum.
const TimetableEntryColorDefault = "BLUE"

type Timetable struct {
	ID        int64
	UserID    int64
	CourseID  int64
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
