package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.CourseRepository = &MySQLCourseRepository{}

type MySQLCourseRepository struct {
	DB *sql.DB
}

func NewMySQLCourseRepository(db *sql.DB) repository.CourseRepository {
	return &MySQLCourseRepository{DB: db}
}

const courseColumns = `c.id, c.room_id, c.day_of_week, c.period, c.teacher_name, c.course_name, c.year, c.semester, c.dedup_key, c.created_at, c.updated_at`

// SaveCourseWithRoom creates a Room (type=course) and a Course in one transaction,
// mirroring MySQLCommunityRepository.SaveCommunityWithRoom. No room_users row is
// created: course rooms are open to any authenticated student.
func (r *MySQLCourseRepository) SaveCourseWithRoom(ctx context.Context, param repository.SaveCourseParam) (*model.Course, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	nowUnix := now.Unix()

	roomResult, err := tx.ExecContext(ctx,
		`INSERT INTO rooms (name, type, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		param.CourseName, model.RoomTypeCourse, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	roomID, err := roomResult.LastInsertId()
	if err != nil {
		return nil, err
	}

	courseResult, err := tx.ExecContext(ctx,
		`INSERT INTO courses (room_id, day_of_week, period, teacher_name, course_name, year, semester, dedup_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		roomID, param.DayOfWeek, param.Period, param.TeacherName, param.CourseName, param.Year, param.Semester, param.DedupKey, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	courseID, err := courseResult.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.Course{
		ID:          courseID,
		RoomID:      roomID,
		DayOfWeek:   param.DayOfWeek,
		Period:      param.Period,
		TeacherName: param.TeacherName,
		CourseName:  param.CourseName,
		Year:        param.Year,
		Semester:    param.Semester,
		DedupKey:    param.DedupKey,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (r *MySQLCourseRepository) FindByDedupKey(ctx context.Context, dedupKey string) (*model.Course, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses c WHERE c.dedup_key = ?`, dedupKey)
	return scanCourse(row)
}

func (r *MySQLCourseRepository) GetCourseByID(ctx context.Context, id int64) (*model.Course, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses c WHERE c.id = ?`, id)
	return scanCourse(row)
}

func (r *MySQLCourseRepository) GetCourseByRoomID(ctx context.Context, roomID int64) (*model.Course, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses c WHERE c.room_id = ?`, roomID)
	return scanCourse(row)
}

func (r *MySQLCourseRepository) SearchByDayPeriod(ctx context.Context, dayOfWeek string, period int, keyword string, year int, semester string, limit, offset int) ([]*model.Course, int, error) {
	searchParam := "%" + keyword + "%"

	var total int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM courses c WHERE c.day_of_week = ? AND c.period = ? AND c.year = ? AND c.semester = ? AND (c.course_name LIKE ? OR c.teacher_name LIKE ?)`,
		dayOfWeek, period, year, semester, searchParam, searchParam,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT `+courseColumns+` FROM courses c
		 WHERE c.day_of_week = ? AND c.period = ? AND c.year = ? AND c.semester = ? AND (c.course_name LIKE ? OR c.teacher_name LIKE ?)
		 ORDER BY c.course_name
		 LIMIT ? OFFSET ?`,
		dayOfWeek, period, year, semester, searchParam, searchParam, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items, err := scanCourses(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListDistinctYears returns every distinct year present in courses, newest first.
func (r *MySQLCourseRepository) ListDistinctYears(ctx context.Context) ([]int, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT DISTINCT c.year FROM courses c ORDER BY c.year DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var years []int
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			return nil, err
		}
		years = append(years, y)
	}
	return years, rows.Err()
}

// ListCourses returns courses matching the optional filters (admin course listing),
// ordered newest-first by year/semester then by day/period for stable paging.
func (r *MySQLCourseRepository) ListCourses(ctx context.Context, param repository.ListCoursesParam) ([]*model.Course, int, error) {
	var where []string
	var args []any

	if param.Year != nil {
		where = append(where, "c.year = ?")
		args = append(args, *param.Year)
	}
	if param.Semester != nil {
		where = append(where, "c.semester = ?")
		args = append(args, *param.Semester)
	}
	if param.DayOfWeek != nil {
		where = append(where, "c.day_of_week = ?")
		args = append(args, *param.DayOfWeek)
	}
	if param.Keyword != "" {
		where = append(where, "(c.course_name LIKE ? OR c.teacher_name LIKE ?)")
		searchParam := "%" + param.Keyword + "%"
		args = append(args, searchParam, searchParam)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM courses c "+whereClause, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append(append([]any{}, args...), param.Limit, param.Offset)
	rows, err := r.DB.QueryContext(ctx,
		`SELECT `+courseColumns+` FROM courses c `+whereClause+`
		 ORDER BY c.year DESC, c.semester DESC, c.day_of_week, c.period, c.course_name
		 LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items, err := scanCourses(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type courseScanner interface {
	Scan(dest ...any) error
}

func scanCourse(row courseScanner) (*model.Course, error) {
	var c model.Course
	var createdAt, updatedAt int64
	if err := row.Scan(&c.ID, &c.RoomID, &c.DayOfWeek, &c.Period, &c.TeacherName, &c.CourseName, &c.Year, &c.Semester, &c.DedupKey, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)
	return &c, nil
}

func scanCourses(rows *sql.Rows) ([]*model.Course, error) {
	var list []*model.Course
	for rows.Next() {
		var c model.Course
		var createdAt, updatedAt int64
		if err := rows.Scan(&c.ID, &c.RoomID, &c.DayOfWeek, &c.Period, &c.TeacherName, &c.CourseName, &c.Year, &c.Semester, &c.DedupKey, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = time.Unix(createdAt, 0)
		c.UpdatedAt = time.Unix(updatedAt, 0)
		list = append(list, &c)
	}
	return list, rows.Err()
}
