package mysql

import (
	"context"
	"database/sql"
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
		`INSERT INTO timetables (user_id, course_id, is_profile_visible, created_at, updated_at) VALUES (?, ?, TRUE, ?, ?)`,
		userID, courseID, nowUnix, nowUnix,
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
		ID:               id,
		UserID:           userID,
		CourseID:         courseID,
		IsProfileVisible: true,
		CreatedAt:        now,
		UpdatedAt:        now,
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

func (r *MySQLTimetableRepository) SetProfileVisibility(ctx context.Context, id, userID int64, visible bool) (*model.Timetable, error) {
	now := time.Now()
	result, err := r.DB.ExecContext(ctx,
		`UPDATE timetables SET is_profile_visible = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		visible, now.Unix(), id, userID,
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
		`SELECT id, user_id, course_id, is_profile_visible, created_at, updated_at FROM timetables WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	return scanTimetable(row)
}

func (r *MySQLTimetableRepository) ListByUser(ctx context.Context, userID int64, year int, semester string) ([]*repository.TimetableEntryWithCourse, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT t.id, t.user_id, t.course_id, t.is_profile_visible, t.created_at, t.updated_at,
		       `+courseColumns+`
		FROM timetables t
		JOIN courses c ON t.course_id = c.id
		WHERE t.user_id = ? AND c.year = ? AND c.semester = ?
		ORDER BY FIELD(c.day_of_week, '月', '火', '水', '木', '金', '土'), c.period
	`, userID, year, semester)
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
			&t.ID, &t.UserID, &t.CourseID, &t.IsProfileVisible, &tCreatedAt, &tUpdatedAt,
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

type timetableScanner interface {
	Scan(dest ...any) error
}

func scanTimetable(row timetableScanner) (*model.Timetable, error) {
	var t model.Timetable
	var createdAt, updatedAt int64
	if err := row.Scan(&t.ID, &t.UserID, &t.CourseID, &t.IsProfileVisible, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	t.CreatedAt = time.Unix(createdAt, 0)
	t.UpdatedAt = time.Unix(updatedAt, 0)
	return &t, nil
}
