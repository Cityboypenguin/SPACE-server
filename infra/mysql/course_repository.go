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

const courseColumns = `c.id, c.room_id, c.day_of_week, c.period, c.teacher_name, c.course_name, c.year, c.semester, c.dedup_key, c.source, c.source_ref, c.source_name, c.discontinued_at, c.created_at, c.updated_at`

// courseRow は courseColumns の並びで1行を受け取る入れ物。courses を JOIN して
// 読む箇所（時間割など）でも同じ並びで受けられるように、宛先の組み立てを1か所にまとめてある。
type courseRow struct {
	c                    model.Course
	createdAt, updatedAt int64
	sourceRef            sql.NullString
	sourceName           sql.NullString
	discontinuedAt       sql.NullInt64
}

func (r *courseRow) dest() []any {
	return []any{&r.c.ID, &r.c.RoomID, &r.c.DayOfWeek, &r.c.Period, &r.c.TeacherName, &r.c.CourseName,
		&r.c.Year, &r.c.Semester, &r.c.DedupKey, &r.c.Source, &r.sourceRef, &r.sourceName, &r.discontinuedAt,
		&r.createdAt, &r.updatedAt}
}

func (r *courseRow) course() *model.Course {
	c := r.c
	c.SourceRef = r.sourceRef.String
	c.SourceName = r.sourceName.String
	if r.discontinuedAt.Valid {
		at := time.Unix(r.discontinuedAt.Int64, 0)
		c.DiscontinuedAt = &at
	}
	c.CreatedAt = time.Unix(r.createdAt, 0)
	c.UpdatedAt = time.Unix(r.updatedAt, 0)
	return &c
}

// nullString は空文字を NULL として書く（source_ref / source_name は「無い」を NULL で持つ）。
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

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
		`INSERT INTO courses (room_id, day_of_week, period, teacher_name, course_name, year, semester, dedup_key, source, source_ref, source_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		roomID, param.DayOfWeek, param.Period, param.TeacherName, param.CourseName, param.Year, param.Semester, param.DedupKey,
		param.Source, nullString(param.SourceRef), nullString(param.SourceName), nowUnix, nowUnix,
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
		Source:      param.Source,
		SourceRef:   param.SourceRef,
		SourceName:  param.SourceName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// courseRoomInsertColumns / courseInsertColumns は一括 INSERT の1行あたりの列数（分割の単位）。
const (
	courseRoomInsertColumns = 4
	courseInsertColumns     = 13
)

// SaveCoursesWithRooms は rooms と courses をそれぞれ1本の INSERT でまとめて作る。
//
// 以前は取り込みが1件ごとに SaveCourseWithRoom を呼んでおり、シラバス1年ぶん
// （数千件）の取り込みでトランザクションと INSERT が件数ぶん並んでいた。
// 取り込みは管理者が眺めている間に走るので、往復の数がそのまま待ち時間になる。
//
// ctx にトランザクションが乗っていればそれに参加する（シラバス同期は更新・廃止・作成を
// 1つのトランザクションで行う）。
//
// 全件を1つのトランザクションに入れているのは、SaveCourseWithRoom と同じく
// 「rooms だけ出来て courses が無い」を作らないため。途中で失敗したら丸ごと
// 巻き戻る（取り込みは再実行できる＝DedupKey で冪等なので、部分適用を残すより
// 何も残さないほうが後始末が要らない）。
func (r *MySQLCourseRepository) SaveCoursesWithRooms(ctx context.Context, params []repository.SaveCourseParam) ([]*model.Course, error) {
	if len(params) == 0 {
		return nil, nil
	}

	now := time.Now()
	nowUnix := now.Unix()

	courses := make([]*model.Course, 0, len(params))
	for _, p := range params {
		courses = append(courses, &model.Course{
			DayOfWeek:   p.DayOfWeek,
			Period:      p.Period,
			TeacherName: p.TeacherName,
			CourseName:  p.CourseName,
			Year:        p.Year,
			Semester:    p.Semester,
			DedupKey:    p.DedupKey,
			Source:      p.Source,
			SourceRef:   p.SourceRef,
			SourceName:  p.SourceName,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	if err := inTx(ctx, r.DB, func(ctx context.Context) error {
		return saveCoursesWithRooms(ctx, extractDB(ctx, r.DB), courses, nowUnix)
	}); err != nil {
		return nil, err
	}
	return courses, nil
}

func saveCoursesWithRooms(ctx context.Context, tx dbtx, courses []*model.Course, nowUnix int64) error {
	// rooms を先に作って採番を受け取る。行数が事前に分かる複数 VALUES の INSERT では
	// InnoDB が AUTO_INCREMENT を連番でまとめて確保するので、先頭IDから順に振ってよい
	// （media の CreateMediaBatch / notifications の SaveBatch と同じ前提）。
	if err := inChunks(courses, courseRoomInsertColumns, func(chunk []*model.Course) error {
		args := make([]any, 0, len(chunk)*courseRoomInsertColumns)
		for _, c := range chunk {
			args = append(args, c.CourseName, model.RoomTypeCourse, nowUnix, nowUnix)
		}
		result, err := tx.ExecContext(ctx,
			`INSERT INTO rooms (name, type, created_at, updated_at) VALUES `+
				valuesPlaceholders(len(chunk), courseRoomInsertColumns), args...)
		if err != nil {
			return err
		}
		firstID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		for i, c := range chunk {
			c.RoomID = firstID + int64(i)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := inChunks(courses, courseInsertColumns, func(chunk []*model.Course) error {
		args := make([]any, 0, len(chunk)*courseInsertColumns)
		for _, c := range chunk {
			args = append(args, c.RoomID, c.DayOfWeek, c.Period, c.TeacherName, c.CourseName,
				c.Year, c.Semester, c.DedupKey, c.Source, nullString(c.SourceRef), nullString(c.SourceName), nowUnix, nowUnix)
		}
		result, err := tx.ExecContext(ctx,
			`INSERT INTO courses (room_id, day_of_week, period, teacher_name, course_name, year, semester, dedup_key, source, source_ref, source_name, created_at, updated_at)
			 VALUES `+valuesPlaceholders(len(chunk), courseInsertColumns), args...)
		if err != nil {
			return err
		}
		firstID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		for i, c := range chunk {
			c.ID = firstID + int64(i)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
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

// SearchByDayPeriod includes 通年 (full-year) courses alongside exact matches on
// semester: a 通年 course occupies its slot in both terms, so it must be visible
// (and registerable) whichever term the student is currently searching in.
//
// 廃止済み（シラバスから消えた）授業は出さない。新しく登録する人を作らないため。
// 既に登録している人の時間割には残る（ListByUser は絞らない）。
func (r *MySQLCourseRepository) SearchByDayPeriod(ctx context.Context, dayOfWeek string, period int, keyword string, year int, semester string, q repository.PageQuery) ([]*model.Course, int, error) {
	searchParam := "%" + keyword + "%"

	total, err := countForPage(ctx, r.DB, q, `SELECT COUNT(*) FROM courses c WHERE c.day_of_week = ? AND c.period = ? AND c.year = ? AND (c.semester = ? OR c.semester = ?) AND c.discontinued_at IS NULL AND (c.course_name LIKE ? OR c.teacher_name LIKE ?)`,
		dayOfWeek, period, year, semester, model.SemesterFull, searchParam, searchParam)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT `+courseColumns+` FROM courses c
		 WHERE c.day_of_week = ? AND c.period = ? AND c.year = ? AND (c.semester = ? OR c.semester = ?) AND c.discontinued_at IS NULL AND (c.course_name LIKE ? OR c.teacher_name LIKE ?)
		 ORDER BY c.course_name, c.id
		 LIMIT ? OFFSET ?`,
		dayOfWeek, period, year, semester, model.SemesterFull, searchParam, searchParam, q.Limit, q.Offset,
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

func (r *MySQLCourseRepository) ListDedupKeysByYear(ctx context.Context, year int) (map[string]bool, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT dedup_key FROM courses WHERE year = ?`, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make(map[string]bool)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys[k] = true
	}
	return keys, rows.Err()
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

	total, err := countForPage(ctx, r.DB, param.Page, "SELECT COUNT(*) FROM courses c "+whereClause, args...)
	if err != nil {
		return nil, 0, err
	}

	queryArgs := append(append([]any{}, args...), param.Page.Limit, param.Page.Offset)
	rows, err := r.DB.QueryContext(ctx,
		`SELECT `+courseColumns+` FROM courses c `+whereClause+`
		 ORDER BY c.year DESC, c.semester DESC, c.day_of_week, c.period, c.course_name, c.id
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
	var r courseRow
	if err := row.Scan(r.dest()...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r.course(), nil
}

func scanCourses(rows *sql.Rows) ([]*model.Course, error) {
	var list []*model.Course
	for rows.Next() {
		var r courseRow
		if err := rows.Scan(r.dest()...); err != nil {
			return nil, err
		}
		list = append(list, r.course())
	}
	return list, rows.Err()
}
