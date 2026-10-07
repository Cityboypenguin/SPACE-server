package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.CourseSyncRepository = &MySQLCourseSyncRepository{}

type MySQLCourseSyncRepository struct {
	DB *sql.DB
}

func NewMySQLCourseSyncRepository(db *sql.DB) repository.CourseSyncRepository {
	return &MySQLCourseSyncRepository{DB: db}
}

func (r *MySQLCourseSyncRepository) ListSyncableCourses(ctx context.Context, year int) ([]*model.Course, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx,
		`SELECT `+courseColumns+` FROM courses c
		 WHERE c.year = ? AND c.source = ? AND c.source_ref IS NOT NULL
		 ORDER BY c.id`,
		year, model.CourseSourceSenshu)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCourses(rows)
}

// UpdateCourses は1件ずつ UPDATE する。同期で変わる授業は数件〜数十件で、列ごとに
// 値の違う一括 UPDATE を組むほどの量にはならない。
func (r *MySQLCourseSyncRepository) UpdateCourses(ctx context.Context, params []repository.CourseUpdateParam) error {
	db := extractDB(ctx, r.DB)
	now := time.Now().Unix()
	for _, p := range params {
		query := `UPDATE courses
			SET semester = ?, day_of_week = ?, period = ?, course_name = ?, teacher_name = ?,
			    source_ref = ?, source_name = ?, dedup_key = ?, updated_at = ?`
		if p.Restore {
			query += `, discontinued_at = NULL`
		}
		query += ` WHERE id = ?`
		if _, err := db.ExecContext(ctx, query,
			p.Semester, p.DayOfWeek, p.Period, p.CourseName, p.TeacherName,
			nullString(p.SourceRef), nullString(p.SourceName), p.DedupKey, now, p.ID,
		); err != nil {
			return err
		}
		if p.RenameRoom {
			if _, err := db.ExecContext(ctx,
				`UPDATE rooms SET name = ?, updated_at = ? WHERE id = ?`, p.CourseName, now, p.RoomID,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *MySQLCourseSyncRepository) DiscontinueCourses(ctx context.Context, ids []int64, at time.Time) error {
	db := extractDB(ctx, r.DB)
	return inChunks(ids, 1, func(chunk []int64) error {
		args := append([]any{at.Unix(), at.Unix()}, int64Args(chunk)...)
		_, err := db.ExecContext(ctx,
			`UPDATE courses SET discontinued_at = ?, updated_at = ?
			 WHERE discontinued_at IS NULL AND id IN (`+inPlaceholders(len(chunk))+`)`, args...)
		return err
	})
}

func (r *MySQLCourseSyncRepository) RetireCourses(ctx context.Context, ids []int64, at time.Time) error {
	db := extractDB(ctx, r.DB)
	return inChunks(ids, 1, func(chunk []int64) error {
		args := append([]any{at.Unix(), at.Unix()}, int64Args(chunk)...)
		// dedup_key は unique なので、同じ値で新しい授業を作れるよう id 付きで退避する。
		// 既に廃止済みの行は、廃止した日時を上書きしない。
		_, err := db.ExecContext(ctx,
			`UPDATE courses
			 SET discontinued_at = COALESCE(discontinued_at, ?),
			     source_ref = NULL,
			     dedup_key = CONCAT('retired:', id, ':', dedup_key),
			     updated_at = ?
			 WHERE id IN (`+inPlaceholders(len(chunk))+`)`, args...)
		return err
	})
}

func (r *MySQLCourseSyncRepository) CountRegistrations(ctx context.Context, courseIDs []int64) (map[int64]int, error) {
	counts := make(map[int64]int)
	db := extractDB(ctx, r.DB)
	if err := inChunks(courseIDs, 1, func(chunk []int64) error {
		rows, err := db.QueryContext(ctx,
			`SELECT course_id, COUNT(*) FROM timetables
			 WHERE course_id IN (`+inPlaceholders(len(chunk))+`)
			 GROUP BY course_id`, int64Args(chunk)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				return err
			}
			counts[id] = n
		}
		return rows.Err()
	}); err != nil {
		return nil, err
	}
	return counts, nil
}

// FindSlotConflicts の「重なる学期」は、時間割の表示と同じく 通年 を両方の学期に
// 数える（通年の授業は前期にも後期にも並ぶ）。
func (r *MySQLCourseSyncRepository) FindSlotConflicts(ctx context.Context, movedCourseIDs []int64) ([]repository.SlotConflict, error) {
	if len(movedCourseIDs) == 0 {
		return nil, nil
	}
	moved := make(map[int64]bool, len(movedCourseIDs))
	for _, id := range movedCourseIDs {
		moved[id] = true
	}

	type pair struct {
		entryID, userID, courseID   int64
		otherEntryID, otherCourseID int64
		otherCourseName             string
	}
	var pairs []pair
	db := extractDB(ctx, r.DB)
	if err := inChunks(movedCourseIDs, 1, func(chunk []int64) error {
		rows, err := db.QueryContext(ctx,
			`SELECT t.id, t.user_id, t.course_id, o.id, o.course_id, oc.course_name
			 FROM timetables t
			 JOIN courses c ON c.id = t.course_id
			 JOIN timetables o ON o.user_id = t.user_id AND o.id <> t.id
			 JOIN courses oc ON oc.id = o.course_id
			 WHERE t.course_id IN (`+inPlaceholders(len(chunk))+`)
			   AND oc.year = c.year AND oc.day_of_week = c.day_of_week AND oc.period = c.period
			   AND (oc.semester = c.semester OR oc.semester = ? OR c.semester = ?)
			 ORDER BY t.id, o.id`,
			append(int64Args(chunk), model.SemesterFull, model.SemesterFull)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p pair
			if err := rows.Scan(&p.entryID, &p.userID, &p.courseID, &p.otherEntryID, &p.otherCourseID, &p.otherCourseName); err != nil {
				return err
			}
			pairs = append(pairs, p)
		}
		return rows.Err()
	}); err != nil {
		return nil, err
	}

	// 外すのは「動いた授業」の登録だけ。相手が動いていない授業ならこちらを外す。
	// どちらも動いた授業なら、先に登録した方（timetables.id の小さい方）を残す。
	var out []repository.SlotConflict
	removed := make(map[int64]bool)
	for _, p := range pairs {
		if removed[p.entryID] || removed[p.otherEntryID] {
			continue
		}
		if moved[p.otherCourseID] && p.otherEntryID > p.entryID {
			continue // 相手の行として後で外される
		}
		removed[p.entryID] = true
		out = append(out, repository.SlotConflict{
			TimetableID:    p.entryID,
			UserID:         p.userID,
			CourseID:       p.courseID,
			KeptCourseID:   p.otherCourseID,
			KeptCourseName: p.otherCourseName,
		})
	}
	return out, nil
}

func (r *MySQLCourseSyncRepository) DeleteTimetableEntries(ctx context.Context, ids []int64) error {
	db := extractDB(ctx, r.DB)
	return inChunks(ids, 1, func(chunk []int64) error {
		_, err := db.ExecContext(ctx,
			`DELETE FROM timetables WHERE id IN (`+inPlaceholders(len(chunk))+`)`, int64Args(chunk)...)
		return err
	})
}

// Savepoint / RollbackToSavepoint は ctx のトランザクションの中でだけ意味を持つ
// （トランザクションの外で呼ぶと MySQL が何もしないか、エラーになる）。
// name は呼び出し側の固定値だけを渡すこと（識別子なのでプレースホルダが使えない）。
func (r *MySQLCourseSyncRepository) Savepoint(ctx context.Context, name string) error {
	if _, ok := txFromContext(ctx); !ok {
		return errors.New("savepoint requires a transaction")
	}
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, "SAVEPOINT "+name)
	return err
}

func (r *MySQLCourseSyncRepository) RollbackToSavepoint(ctx context.Context, name string) error {
	if _, ok := txFromContext(ctx); !ok {
		return errors.New("savepoint requires a transaction")
	}
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
	return err
}

const courseSyncReviewColumns = `id, year, kind, fingerprint, status, message, existing_json, proposed_json,
	first_run_id, applied_run_id, resolved_by, resolved_at, created_at, updated_at`

func (r *MySQLCourseSyncRepository) ListReviewsByYear(ctx context.Context, year int) ([]*model.CourseSyncReview, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx,
		`SELECT `+courseSyncReviewColumns+` FROM course_sync_reviews WHERE year = ? ORDER BY id`, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCourseSyncReviews(rows)
}

// UpsertReview は fingerprint が同じ確認があれば内容を新しくして確認待ちへ戻す
// （APPLIED 後に同じ状況が再び起きた場合など）。照合は判断済み（SAME / DIFFERENT /
// IGNORED）の fingerprint では確認を作らないので、ここで判断が上書きされることは無い。
func (r *MySQLCourseSyncRepository) UpsertReview(ctx context.Context, review *model.CourseSyncReview) (int64, error) {
	existing, err := json.Marshal(review.Existing)
	if err != nil {
		return 0, err
	}
	proposed, err := json.Marshal(review.Proposed)
	if err != nil {
		return 0, err
	}
	result, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT INTO course_sync_reviews
		   (year, kind, fingerprint, status, message, existing_json, proposed_json, first_run_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   id = LAST_INSERT_ID(id),
		   status = VALUES(status),
		   message = VALUES(message),
		   existing_json = VALUES(existing_json),
		   proposed_json = VALUES(proposed_json),
		   applied_run_id = NULL,
		   resolved_by = NULL,
		   resolved_at = NULL,
		   updated_at = VALUES(updated_at)`,
		review.Year, string(review.Kind), review.Fingerprint, string(review.Status), review.Message,
		existing, proposed, review.FirstRunID, review.CreatedAt.Unix(), review.UpdatedAt.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *MySQLCourseSyncRepository) MarkReviewsApplied(ctx context.Context, ids []int64, runID int64, at time.Time) error {
	db := extractDB(ctx, r.DB)
	return inChunks(ids, 1, func(chunk []int64) error {
		args := append([]any{string(model.CourseSyncReviewApplied), runID, at.Unix()}, int64Args(chunk)...)
		_, err := db.ExecContext(ctx,
			`UPDATE course_sync_reviews SET status = ?, applied_run_id = ?, updated_at = ?
			 WHERE id IN (`+inPlaceholders(len(chunk))+`)`, args...)
		return err
	})
}

func (r *MySQLCourseSyncRepository) ListReviews(ctx context.Context, param repository.ListCourseSyncReviewsParam) ([]*model.CourseSyncReview, int, error) {
	var where []string
	var args []any
	if param.Year != nil {
		where = append(where, "year = ?")
		args = append(args, *param.Year)
	}
	if param.Status != nil {
		where = append(where, "status = ?")
		args = append(args, string(*param.Status))
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	db := extractDB(ctx, r.DB)
	total, err := countForPage(ctx, db, param.Page, `SELECT COUNT(*) FROM course_sync_reviews `+whereClause, args...)
	if err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT `+courseSyncReviewColumns+` FROM course_sync_reviews `+whereClause+`
		 ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, param.Page.Limit, param.Page.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanCourseSyncReviews(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *MySQLCourseSyncRepository) GetReview(ctx context.Context, id int64) (*model.CourseSyncReview, error) {
	rows, err := extractDB(ctx, r.DB).QueryContext(ctx,
		`SELECT `+courseSyncReviewColumns+` FROM course_sync_reviews WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanCourseSyncReviews(rows)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return items[0], nil
}

func (r *MySQLCourseSyncRepository) ResolveReview(ctx context.Context, id int64, status model.CourseSyncReviewStatus, resolvedBy int64, at time.Time) (bool, error) {
	result, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`UPDATE course_sync_reviews SET status = ?, resolved_by = ?, resolved_at = ?, updated_at = ?
		 WHERE id = ? AND status = ?`,
		string(status), resolvedBy, at.Unix(), at.Unix(), id, string(model.CourseSyncReviewPending))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func scanCourseSyncReviews(rows *sql.Rows) ([]*model.CourseSyncReview, error) {
	var list []*model.CourseSyncReview
	for rows.Next() {
		var rv model.CourseSyncReview
		var kind, status string
		var existing, proposed []byte
		var firstRunID, appliedRunID, resolvedBy, resolvedAt sql.NullInt64
		var createdAt, updatedAt int64
		if err := rows.Scan(&rv.ID, &rv.Year, &kind, &rv.Fingerprint, &status, &rv.Message, &existing, &proposed,
			&firstRunID, &appliedRunID, &resolvedBy, &resolvedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		rv.Kind = model.CourseSyncReviewKind(kind)
		rv.Status = model.CourseSyncReviewStatus(status)
		if err := json.Unmarshal(existing, &rv.Existing); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(proposed, &rv.Proposed); err != nil {
			return nil, err
		}
		rv.FirstRunID = nullInt64Ptr(firstRunID)
		rv.AppliedRunID = nullInt64Ptr(appliedRunID)
		rv.ResolvedBy = nullInt64Ptr(resolvedBy)
		if resolvedAt.Valid {
			at := time.Unix(resolvedAt.Int64, 0)
			rv.ResolvedAt = &at
		}
		rv.CreatedAt = time.Unix(createdAt, 0)
		rv.UpdatedAt = time.Unix(updatedAt, 0)
		list = append(list, &rv)
	}
	return list, rows.Err()
}

const courseSyncRunColumns = `id, year, dry_run, triggered_by, site_total, listed_rows, unidentified_rows,
	created_count, updated_count, discontinued_count, restored_count, review_count, unchanged_count,
	unregistered_count, discontinue_skipped_reason, started_at, finished_at`

func (r *MySQLCourseSyncRepository) SaveRun(ctx context.Context, run *model.CourseSyncRun) (int64, error) {
	result, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`INSERT INTO course_sync_runs
		   (year, dry_run, triggered_by, site_total, listed_rows, unidentified_rows,
		    created_count, updated_count, discontinued_count, restored_count, review_count, unchanged_count,
		    unregistered_count, discontinue_skipped_reason, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.Year, run.DryRun, run.TriggeredBy, run.SiteTotal, run.ListedRows, run.UnidentifiedRows,
		run.Created, run.Updated, run.Discontinued, run.Restored, run.Reviews, run.Unchanged, run.Unregistered,
		nullString(run.DiscontinueSkippedReason), run.StartedAt.Unix(), run.FinishedAt.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// courseSyncChangeInsertColumns は course_sync_changes に1行入れるときの列数（分割の単位）。
const courseSyncChangeInsertColumns = 11

func (r *MySQLCourseSyncRepository) SaveChanges(ctx context.Context, runID int64, changes []*model.CourseSyncChange) error {
	db := extractDB(ctx, r.DB)
	return inChunks(changes, courseSyncChangeInsertColumns, func(chunk []*model.CourseSyncChange) error {
		args := make([]any, 0, len(chunk)*courseSyncChangeInsertColumns)
		for _, c := range chunk {
			before, err := marshalSnapshot(c.Before)
			if err != nil {
				return err
			}
			after, err := marshalSnapshot(c.After)
			if err != nil {
				return err
			}
			args = append(args, runID, string(c.Kind), c.CourseID, c.ReviewID, truncateRunes(c.CourseName, 255),
				truncateRunes(c.TeacherName, 255), truncateRunes(c.Detail, 1000), before, after,
				c.RegisteredCount, c.UnregisteredCount)
		}
		_, err := db.ExecContext(ctx,
			`INSERT INTO course_sync_changes
			   (run_id, kind, course_id, review_id, course_name, teacher_name, detail, before_json, after_json,
			    registered_count, unregistered_count)
			 VALUES `+valuesPlaceholders(len(chunk), courseSyncChangeInsertColumns), args...)
		return err
	})
}

func (r *MySQLCourseSyncRepository) ListRuns(ctx context.Context, year *int, page repository.PageQuery) ([]*model.CourseSyncRun, int, error) {
	whereClause := ""
	var args []any
	if year != nil {
		whereClause = "WHERE year = ?"
		args = append(args, *year)
	}
	db := extractDB(ctx, r.DB)
	total, err := countForPage(ctx, db, page, `SELECT COUNT(*) FROM course_sync_runs `+whereClause, args...)
	if err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT `+courseSyncRunColumns+` FROM course_sync_runs `+whereClause+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []*model.CourseSyncRun
	for rows.Next() {
		run, err := scanCourseSyncRun(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, run)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *MySQLCourseSyncRepository) GetRun(ctx context.Context, id int64) (*model.CourseSyncRun, error) {
	row := extractDB(ctx, r.DB).QueryRowContext(ctx,
		`SELECT `+courseSyncRunColumns+` FROM course_sync_runs WHERE id = ?`, id)
	run, err := scanCourseSyncRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

func scanCourseSyncRun(row courseScanner) (*model.CourseSyncRun, error) {
	var run model.CourseSyncRun
	var triggeredBy sql.NullInt64
	var reason sql.NullString
	var startedAt, finishedAt int64
	if err := row.Scan(&run.ID, &run.Year, &run.DryRun, &triggeredBy, &run.SiteTotal, &run.ListedRows, &run.UnidentifiedRows,
		&run.Created, &run.Updated, &run.Discontinued, &run.Restored, &run.Reviews, &run.Unchanged,
		&run.Unregistered, &reason, &startedAt, &finishedAt); err != nil {
		return nil, err
	}
	run.TriggeredBy = nullInt64Ptr(triggeredBy)
	run.DiscontinueSkippedReason = reason.String
	run.StartedAt = time.Unix(startedAt, 0)
	run.FinishedAt = time.Unix(finishedAt, 0)
	return &run, nil
}

func (r *MySQLCourseSyncRepository) ListChanges(ctx context.Context, runID int64, kind *model.CourseSyncChangeKind, page repository.PageQuery) ([]*model.CourseSyncChange, int, error) {
	whereClause := "WHERE run_id = ?"
	args := []any{runID}
	if kind != nil {
		whereClause += " AND kind = ?"
		args = append(args, string(*kind))
	}
	db := extractDB(ctx, r.DB)
	total, err := countForPage(ctx, db, page, `SELECT COUNT(*) FROM course_sync_changes `+whereClause, args...)
	if err != nil {
		return nil, 0, err
	}
	// 影響の大きい（登録者の多い）変更から並べる。管理者はそこから確かめたい。
	rows, err := db.QueryContext(ctx,
		`SELECT id, run_id, kind, course_id, review_id, course_name, teacher_name, detail, before_json, after_json,
		        registered_count, unregistered_count
		 FROM course_sync_changes `+whereClause+`
		 ORDER BY registered_count DESC, id
		 LIMIT ? OFFSET ?`,
		append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*model.CourseSyncChange
	for rows.Next() {
		var c model.CourseSyncChange
		var k string
		var courseID, reviewID sql.NullInt64
		var before, after []byte
		if err := rows.Scan(&c.ID, &c.RunID, &k, &courseID, &reviewID, &c.CourseName, &c.TeacherName, &c.Detail,
			&before, &after, &c.RegisteredCount, &c.UnregisteredCount); err != nil {
			return nil, 0, err
		}
		c.Kind = model.CourseSyncChangeKind(k)
		c.CourseID = nullInt64Ptr(courseID)
		c.ReviewID = nullInt64Ptr(reviewID)
		if c.Before, err = unmarshalSnapshot(before); err != nil {
			return nil, 0, err
		}
		if c.After, err = unmarshalSnapshot(after); err != nil {
			return nil, 0, err
		}
		list = append(list, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func marshalSnapshot(s *model.CourseSnapshot) (any, error) {
	if s == nil {
		return nil, nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func unmarshalSnapshot(b []byte) (*model.CourseSnapshot, error) {
	if b == nil {
		return nil, nil
	}
	var s model.CourseSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// truncateRunes は VARCHAR(n) に収まるよう文字数（バイトではない）で切る。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
