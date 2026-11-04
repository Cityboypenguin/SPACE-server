package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
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

// ■ 時間割の書き込みとシラバス同期の並行
//
// 時間割は1コマ1授業。シラバス同期は授業のコマを書き換え、移った先で重なった登録を
// 外す（infra/mysql/course_sync_repository.go の FindSlotConflicts）。利用者の時間割の
// 書き込みがその最中に走っても重なりが残らないよう、両者は授業の行のロックで順番を付ける。
//
//   - 同期は書き換える授業の行を、ID の昇順に先に FOR UPDATE で押さえる
//   - 時間割の書き込みは、足す授業の行を ID の昇順に FOR SHARE で押さえてからコマを読む
//
// 同期が先なら、書き込みは同期の確定を待って新しいコマで判定する。書き込みが先なら、
// 同期はその確定を待ち、確定した登録を含めて重なりを判定する（同期の判定はロック付きの
// 読み取りなので、確定済みの最新の行を見る）。同じ順で押さえるので授業の行どうしでは
// 行き違わないが、時間割の行で稀に行き違うことはある。そのときは InnoDB が片方を巻き戻すので、
// 書き込み側は retryOnDeadlock でやり直す。

// lockCoursesForShare は courseIDs の授業の行を ID の昇順に共有ロックする。
func lockCoursesForShare(ctx context.Context, tx *sql.Tx, courseIDs []int64) error {
	ids := append([]int64(nil), courseIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return inChunks(ids, 1, func(chunk []int64) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id FROM courses WHERE id IN (`+inPlaceholders(len(chunk))+`) ORDER BY id FOR SHARE`,
			int64Args(chunk)...)
		if err != nil {
			return err
		}
		return rows.Close()
	})
}

// Upsert registers courseID into userID's timetable. Any existing entry occupying
// the same slot for this user is deleted first, so a student only ever has one
// course per slot (F-01/F-02 "overwrite on re-registration").
//
// 「同じコマ」は同じ年度・同じ曜日時限で、学期が重なるもの（同じ学期か、どちらかが
// 通年）。以前は年度も学期も見ずに曜日時限だけで消していたので、前期の水4に登録すると
// 去年の授業や後期の水4の登録まで消えていた。
func (r *MySQLTimetableRepository) Upsert(ctx context.Context, userID, courseID int64) (*model.Timetable, error) {
	return retryOnDeadlock(func() (*model.Timetable, error) {
		return r.upsert(ctx, userID, courseID)
	})
}

func (r *MySQLTimetableRepository) upsert(ctx context.Context, userID, courseID int64) (*model.Timetable, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// FOR SHARE: 同期がこの授業のコマを書き換えている最中なら、確定を待って新しいコマを読む。
	var dayOfWeek, semester string
	var period, year int
	var discontinuedAt sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT day_of_week, period, year, semester, discontinued_at FROM courses WHERE id = ? FOR SHARE`, courseID,
	).Scan(&dayOfWeek, &period, &year, &semester, &discontinuedAt); err != nil {
		return nil, err
	}
	// 廃止済みの授業へ新しく登録させない（シラバスから消えた授業）。
	if discontinuedAt.Valid {
		return nil, repository.ErrCourseDiscontinued
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE t FROM timetables t
		JOIN courses c ON t.course_id = c.id
		WHERE t.user_id = ? AND c.year = ? AND c.day_of_week = ? AND c.period = ?
		  AND (c.semester = ? OR c.semester = ? OR ? = ?)
	`, userID, year, dayOfWeek, period, semester, model.SemesterFull, semester, model.SemesterFull); err != nil {
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
		var c courseRow
		var tCreatedAt, tUpdatedAt int64
		dest := append([]any{&t.ID, &t.UserID, &t.CourseID, &t.Color, &tCreatedAt, &tUpdatedAt}, c.dest()...)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(tCreatedAt, 0)
		t.UpdatedAt = time.Unix(tUpdatedAt, 0)
		list = append(list, &repository.TimetableEntryWithCourse{Timetable: &t, Course: c.course()})
	}
	return list, rows.Err()
}

// timetableInsertColumns は timetables に1行入れるときの列数（分割の単位）。
const timetableInsertColumns = 5

// ReplaceForSemester implements the "edit mode" batch-commit flow: the whole
// desired course list for a semester is applied in one transaction, guarded by an
// optimistic-concurrency check against baselineEntryIDs. See the interface doc
// comment for the full contract.
func (r *MySQLTimetableRepository) ReplaceForSemester(ctx context.Context, userID int64, year int, semester string, baselineEntryIDs, desiredCourseIDs []int64) ([]*repository.TimetableEntryWithCourse, error) {
	if _, err := retryOnDeadlock(func() (struct{}, error) {
		return struct{}{}, r.replaceForSemester(ctx, userID, year, semester, baselineEntryIDs, desiredCourseIDs)
	}); err != nil {
		return nil, err
	}
	return r.ListByUser(ctx, userID, year, semester)
}

func (r *MySQLTimetableRepository) replaceForSemester(ctx context.Context, userID int64, year int, semester string, baselineEntryIDs, desiredCourseIDs []int64) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 足す授業の行を先に押さえる（ファイル冒頭のコメント参照）。以降のコマの読み取りは
	// 同期の確定を待った後の値になる。
	if err := lockCoursesForShare(ctx, tx, desiredCourseIDs); err != nil {
		return err
	}

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
		FOR UPDATE OF t
	`, userID, year, semester, model.SemesterFull)
	if err != nil {
		return err
	}
	currentCourseByEntry := make(map[int64]int64)
	for rows.Next() {
		var entryID, courseID int64
		if err := rows.Scan(&entryID, &courseID); err != nil {
			rows.Close()
			return err
		}
		currentCourseByEntry[entryID] = courseID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(currentCourseByEntry) != len(baselineEntryIDs) {
		return repository.ErrTimetableConflict
	}
	for _, id := range baselineEntryIDs {
		if _, ok := currentCourseByEntry[id]; !ok {
			return repository.ErrTimetableConflict
		}
	}

	if err := checkNoSlotConflicts(ctx, tx, desiredCourseIDs); err != nil {
		return err
	}

	currentCourseIDs := make(map[int64]bool, len(currentCourseByEntry))
	for _, courseID := range currentCourseByEntry {
		currentCourseIDs[courseID] = true
	}
	desiredCourseSet := make(map[int64]bool, len(desiredCourseIDs))
	for _, courseID := range desiredCourseIDs {
		desiredCourseSet[courseID] = true
	}

	// 消すぶん・足すぶんをそれぞれ1本にまとめる。以前はどちらも for の中で
	// 1件ずつ ExecContext しており、学期まるごと入れ替えると科目数ぶん往復していた
	// （トランザクションの中なので結果は同じで、費用だけが科目数に比例していた）。
	var removedEntryIDs []int64
	for entryID, courseID := range currentCourseByEntry {
		if !desiredCourseSet[courseID] {
			removedEntryIDs = append(removedEntryIDs, entryID)
		}
	}
	var addedCourseIDs []int64
	for _, courseID := range desiredCourseIDs {
		if currentCourseIDs[courseID] {
			continue
		}
		addedCourseIDs = append(addedCourseIDs, courseID)
	}

	// 廃止済みの授業は、既に登録しているぶんは残せるが、新しくは足させない。
	if err := checkNotDiscontinued(ctx, tx, addedCourseIDs); err != nil {
		return err
	}

	if err := inChunks(removedEntryIDs, 1, func(chunk []int64) error {
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		_, err := tx.ExecContext(ctx,
			`DELETE FROM timetables WHERE id IN (`+inPlaceholders(len(chunk))+`)`, args...)
		return err
	}); err != nil {
		return err
	}

	now := time.Now().Unix()
	if err := inChunks(addedCourseIDs, timetableInsertColumns, func(chunk []int64) error {
		args := make([]any, 0, len(chunk)*timetableInsertColumns)
		for _, courseID := range chunk {
			args = append(args, userID, courseID, model.TimetableEntryColorDefault, now, now)
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO timetables (user_id, course_id, color, created_at, updated_at) VALUES `+
				valuesPlaceholders(len(chunk), timetableInsertColumns), args...)
		return err
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// checkNotDiscontinued returns repository.ErrCourseDiscontinued if any of courseIDs
// has been discontinued.
func checkNotDiscontinued(ctx context.Context, tx *sql.Tx, courseIDs []int64) error {
	found := false
	if err := inChunks(courseIDs, 1, func(chunk []int64) error {
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM courses WHERE discontinued_at IS NOT NULL AND id IN (`+inPlaceholders(len(chunk))+`) FOR SHARE`,
			args...).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			found = true
		}
		return nil
	}); err != nil {
		return err
	}
	if found {
		return repository.ErrCourseDiscontinued
	}
	return nil
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
		`SELECT day_of_week, period FROM courses WHERE id IN (`+strings.Join(placeholders, ",")+`) FOR SHARE`,
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

// GetRegisteredAt returns the registration time (timetables.created_at) for
// (userID, courseID), or nil when the course is not in the user's timetable.
// 未読の起点に使うので、登録し直し（Upsert）で行が作り直されたときは
// その新しい時刻が起点になる。
func (r *MySQLTimetableRepository) GetRegisteredAt(ctx context.Context, userID, courseID int64) (*int64, error) {
	var createdAt int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT created_at FROM timetables WHERE user_id = ? AND course_id = ?`,
		userID, courseID,
	).Scan(&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

// ListRegistrantIDsByCourseRoomID は授業ルームの更新を知らせる宛先（履修者のID）を返す。
//
// messages も course_room_reads も JOIN しない。ここが返すのは「誰に知らせるか」
// だけで、未読がいくつかは知らせを受けた本人が自分のぶんだけ数える
// （myCourseRoomUnreadCounts / CountUnreadByCourseRooms）。
//
// 以前はここで履修者ごとの未読数まで数えていた（CountUnreadMessagesPerCourseRegistrant）。
// 送信1件ごとに履修者×メッセージを JOIN・GROUP BY することになり、履修者数百人規模の
// 授業では投稿のたびにその集計が走る。誰も見ていないバッジのために全員ぶんを数えるのは
// 割に合わないので、集計そのものをやめて宛先の列挙だけを残した。
//
// 同じ利用者が同じ授業を二重に登録することは無い（timetables は user_id と
// スロットで一意）ので DISTINCT は付けていない。
func (r *MySQLTimetableRepository) ListRegistrantIDsByCourseRoomID(ctx context.Context, roomID int64) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT t.user_id
		FROM timetables t
		JOIN courses c ON c.id = t.course_id
		WHERE c.room_id = ?
	`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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

// CountByCourseIDs は CountByCourseID の一括版（GROUP BY 1本）。
func (r *MySQLTimetableRepository) CountByCourseIDs(ctx context.Context, courseIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(courseIDs))
	if len(courseIDs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(courseIDs)), ",")
	args := make([]any, len(courseIDs))
	for i, id := range courseIDs {
		args[i] = id
	}

	rows, err := r.DB.QueryContext(ctx, fmt.Sprintf(`
		SELECT course_id, COUNT(*)
		FROM timetables
		WHERE course_id IN (%s)
		GROUP BY course_id
	`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var courseID int64
		var count int
		if err := rows.Scan(&courseID, &count); err != nil {
			return nil, err
		}
		result[courseID] = count
	}
	return result, rows.Err()
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
