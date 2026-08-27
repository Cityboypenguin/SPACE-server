package repository

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// TimetableEntryWithCourse pairs a Timetable row with its Course, as returned by
// list queries that always need both (there is no separate GraphQL field resolver
// for TimetableEntry.course; the presenter embeds it directly).
type TimetableEntryWithCourse struct {
	Timetable *model.Timetable
	Course    *model.Course
}

// ErrTimetableConflict is returned by ReplaceForSemester when the entries currently
// on record no longer match baselineEntryIDs (an optimistic-concurrency check — the
// caller's view of the timetable was taken from a stale baseline, e.g. another
// browser tab already registered/removed something in the meantime).
var ErrTimetableConflict = errors.New("timetable entries changed since baseline was loaded")

// ErrTimetableSlotConflict is returned by ReplaceForSemester when desiredCourseIDs
// contains two courses occupying the same day/period slot.
var ErrTimetableSlotConflict = errors.New("desired courses contain a duplicate day/period slot")

type TimetableRepository interface {
	// Upsert registers courseID into userID's timetable. Any existing registration
	// by the same user for a course occupying the same day/period slot is replaced,
	// implementing the "overwrite on re-registration" behavior from F-01/F-02.
	Upsert(ctx context.Context, userID, courseID int64) (*model.Timetable, error)
	// Remove deletes a timetable entry, scoped to userID so a user can only remove
	// their own entries.
	Remove(ctx context.Context, id, userID int64) (bool, error)
	// SetColor updates the entry's display color on the timetable grid. color must be
	// one of the fixed palette keys enforced by the GraphQL TimetableEntryColor enum.
	SetColor(ctx context.Context, id, userID int64, color string) (*model.Timetable, error)
	ListByUser(ctx context.Context, userID int64, year int, semester string) ([]*TimetableEntryWithCourse, error)
	// IsRegistered reports whether userID has courseID in their timetable — used to
	// gate course-chat writes (message/question/answer/poll) to students who have
	// actually registered for the course, not just any authenticated user.
	IsRegistered(ctx context.Context, userID, courseID int64) (bool, error)
	// ReplaceForSemester atomically replaces userID's timetable entries for
	// (year, semester) with exactly desiredCourseIDs, in one transaction: entries
	// whose course is no longer desired are deleted, courses newly desired are
	// inserted, and entries whose course is unchanged are left untouched
	// (preserving Color). The current entry IDs are compared against
	// baselineEntryIDs (as a set) before any write; a mismatch aborts the whole
	// operation with ErrTimetableConflict instead of silently clobbering changes
	// made elsewhere since the baseline was loaded. Returns ErrTimetableSlotConflict
	// if desiredCourseIDs contains two courses in the same day/period slot.
	ReplaceForSemester(ctx context.Context, userID int64, year int, semester string, baselineEntryIDs, desiredCourseIDs []int64) ([]*TimetableEntryWithCourse, error)
}
