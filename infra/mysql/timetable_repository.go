package mysql

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.TimetableRepository = &MySQLTimetableRepository{}

type MySQLTimetableRepository struct {
	DB *sql.DB
}

func NewMySQLTimetableRepository(db *sql.DB) repository.TimetableRepository {
	return &MySQLTimetableRepository{DB: db}
}

// Upsert registers courseID into userID's timetable. Any existing entry occupying
// the same (day_of_week, period) slot for this user is deleted first, so a student
// only ever has one course per slot (F-01/F-02 "overwrite on re-registration").
func (r *MySQLTimetableRepository) Upsert(ctx context.Context, userID, courseID int64) (*model.Timetable, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var dayOfWeek string
	var period int
	if err := tx.QueryRowContext(ctx, `SELECT day_of_week, period FROM courses WHERE id = ?`, courseID).Scan(&dayOfWeek, &period); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE t FROM timetables t
		JOIN courses c ON t.course_id = c.id
		WHERE t.user_id = ? AND c.day_of_week = ? AND c.period = ?
	`, userID, dayOfWeek, period); err != nil {
		return nil, err
	}

	now := time.Now()
	nowUnix := now.Unix()
	result, err := tx.ExecContext(ctx,
		`INSERT INTO timetables (user_id, course_id, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		userID, courseID, model.TimetableEntryColorDefault, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.Timetable{
		ID:        id,
		UserID:    userID,
		CourseID:  courseID,
		Color:     model.TimetableEntryColorDefault,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (r *MySQLTimetableRepository) Remove(ctx context.Context, id, userID int64) (bool, error) {
	result, err := r.DB.ExecContext(ctx, `DELETE FROM timetables WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetColor updates the entry's display color, scoped to userID so a user can only
// recolor their own entries.
func (r *MySQLTimetableRepository) SetColor(ctx context.Context, id, userID int64, color string) (*model.Timetable, error) {
	now := time.Now()
	result, err := r.DB.ExecContext(ctx,
		`UPDATE timetables SET color = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		color, now.Unix(), id, userID,
	)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}

	row := r.DB.QueryRowContext(ctx,
		`SELECT id, user_id, course_id, color, created_at, updated_at FROM timetables WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	return scanTimetable(row)
}

func (r *MySQLTimetableRepository) ListByUser(ctx context.Context, userID int64, year int, semester string) ([]*repository.TimetableEntryWithCourse, error) {
	// 通年 (full-year) courses have a single courses row for the whole year and
	// must show up in both semester's views, so they're included alongside an
	// exact match on the requested semester rather than only exact matches.
	rows, err := r.DB.QueryContext(ctx, `
		SELECT t.id, t.user_id, t.course_id, t.color, t.created_at, t.updated_at,
		       `+courseColumns+`
		FROM timetables t
		JOIN courses c ON t.course_id = c.id
		WHERE t.user_id = ? AND c.year = ? AND (c.semester = ? OR c.semester = ?)
		ORDER BY FIELD(c.day_of_week, '月', '火', '水', '木', '金', '土'), c.period
	`, userID, year, semester, model.SemesterFull)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*repository.TimetableEntryWithCourse
	for rows.Next() {
		var t model.Timetable
		var c model.Course
		var tCreatedAt, tUpdatedAt, cCreatedAt, cUpdatedAt int64
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.CourseID, &t.Color, &tCreatedAt, &tUpdatedAt,
			&c.ID, &c.RoomID, &c.DayOfWeek, &c.Period, &c.TeacherName, &c.CourseName, &c.Year, &c.Semester, &c.DedupKey, &cCreatedAt, &cUpdatedAt,
		); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(tCreatedAt, 0)
		t.UpdatedAt = time.Unix(tUpdatedAt, 0)
		c.CreatedAt = time.Unix(cCreatedAt, 0)
		c.UpdatedAt = time.Unix(cUpdatedAt, 0)
		list = append(list, &repository.TimetableEntryWithCourse{Timetable: &t, Course: &c})
	}
	return list, rows.Err()
}

// ReplaceForSemester implements the "edit mode" batch-commit flow: the whole
// desired course list for a semester is applied in one transaction, guarded by an
// optimistic-concurrency check against baselineEntryIDs. See the interface doc
// comment for the full contract.
func (r *MySQLTimetableRepository) ReplaceForSemester(ctx context.Context, userID int64, year int, semester string, baselineEntryIDs, desiredCourseIDs []int64) ([]*repository.TimetableEntryWithCourse, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// FOR UPDATE locks the rows for the duration of the transaction so a concurrent
	// ReplaceForSemester call for the same user/semester can't interleave between
	// this check and the writes below. 通年 (full-year) entries are included (same
	// as ListByUser) so the baseline the client diffed against - and the "current
	// entries in this view" set used below - matches what was actually displayed.
	rows, err := tx.QueryContext(ctx, `
		SELECT t.id, t.course_id
		FROM timetables t
		JOIN courses c ON t.course_id = c.id
		WHERE t.user_id = ? AND c.year = ? AND (c.semester = ? OR c.semester = ?)
		FOR UPDATE
	`, userID, year, semester, model.SemesterFull)
	if err != nil {
		return nil, err
	}
	currentCourseByEntry := make(map[int64]int64)
	for rows.Next() {
		var entryID, courseID int64
		if err := rows.Scan(&entryID, &courseID); err != nil {
			rows.Close()
			return nil, err
		}
		currentCourseByEntry[entryID] = courseID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(currentCourseByEntry) != len(baselineEntryIDs) {
		return nil, repository.ErrTimetableConflict
	}
	for _, id := range baselineEntryIDs {
		if _, ok := currentCourseByEntry[id]; !ok {
			return nil, repository.ErrTimetableConflict
		}
	}

	if err := checkNoSlotConflicts(ctx, tx, desiredCourseIDs); err != nil {
		return nil, err
	}

	currentCourseIDs := make(map[int64]bool, len(currentCourseByEntry))
	for _, courseID := range currentCourseByEntry {
		currentCourseIDs[courseID] = true
	}
	desiredCourseSet := make(map[int64]bool, len(desiredCourseIDs))
	for _, courseID := range desiredCourseIDs {
		desiredCourseSet[courseID] = true
	}

	for entryID, courseID := range currentCourseByEntry {
		if !desiredCourseSet[courseID] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM timetables WHERE id = ?`, entryID); err != nil {
				return nil, err
			}
		}
	}

	now := time.Now().Unix()
	for _, courseID := range desiredCourseIDs {
		if currentCourseIDs[courseID] {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO timetables (user_id, course_id, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			userID, courseID, model.TimetableEntryColorDefault, now, now,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.ListByUser(ctx, userID, year, semester)
}

// checkNoSlotConflicts returns repository.ErrTimetableSlotConflict if courseIDs
// contains two courses occupying the same (day_of_week, period) slot.
func checkNoSlotConflicts(ctx context.Context, tx *sql.Tx, courseIDs []int64) error {
	if len(courseIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(courseIDs))
	args := make([]any, len(courseIDs))
	for i, id := range courseIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT day_of_week, period FROM courses WHERE id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	seen := make(map[string]bool, len(courseIDs))
	for rows.Next() {
		var day string
		var period int
		if err := rows.Scan(&day, &period); err != nil {
			return err
		}
		key := day + ":" + strconv.Itoa(period)
		if seen[key] {
			return repository.ErrTimetableSlotConflict
		}
		seen[key] = true
	}
	return rows.Err()
}

func (r *MySQLTimetableRepository) IsRegistered(ctx context.Context, userID, courseID int64) (bool, error) {
	var exists int
	err := r.DB.QueryRowContext(ctx,
		`SELECT 1 FROM timetables WHERE user_id = ? AND course_id = ? LIMIT 1`,
		userID, courseID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *MySQLTimetableRepository) CountByCourseID(ctx context.Context, courseID int64) (int, error) {
	var count int
	err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM timetables WHERE course_id = ?`,
		courseID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

type timetableScanner interface {
	Scan(dest ...any) error
}

func scanTimetable(row timetableScanner) (*model.Timetable, error) {
	var t model.Timetable
	var createdAt, updatedAt int64
	if err := row.Scan(&t.ID, &t.UserID, &t.CourseID, &t.Color, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.CreatedAt = time.Unix(createdAt, 0)
	t.UpdatedAt = time.Unix(updatedAt, 0)
	return &t, nil
}
