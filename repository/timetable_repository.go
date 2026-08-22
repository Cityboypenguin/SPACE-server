package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// TimetableEntryWithCourse pairs a Timetable row with its Course, as returned by
// list queries that always need both (there is no separate GraphQL field resolver
// for TimetableEntry.course; the presenter embeds it directly).
type TimetableEntryWithCourse struct {
	Timetable *model.Timetable
	Course    *model.Course
}

type TimetableRepository interface {
	// Upsert registers courseID into userID's timetable. Any existing registration
	// by the same user for a course occupying the same day/period slot is replaced,
	// implementing the "overwrite on re-registration" behavior from F-01/F-02.
	Upsert(ctx context.Context, userID, courseID int64) (*model.Timetable, error)
	// Remove deletes a timetable entry, scoped to userID so a user can only remove
	// their own entries.
	Remove(ctx context.Context, id, userID int64) (bool, error)
	SetProfileVisibility(ctx context.Context, id, userID int64, visible bool) (*model.Timetable, error)
	ListByUser(ctx context.Context, userID int64, year int, semester string) ([]*TimetableEntryWithCourse, error)
}
