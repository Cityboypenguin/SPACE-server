package mysql

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

func mediaTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, cleanup := throwawaySchemaDB(t, "space_media_test", []string{
		`CREATE TABLE media (
			id BIGINT NOT NULL AUTO_INCREMENT,
			uploader_user_id BIGINT NOT NULL,
			storage_key VARCHAR(255) NOT NULL,
			content_type VARCHAR(100) NOT NULL,
			width INT NULL,
			height INT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE post_media (
			post_id BIGINT NOT NULL,
			media_id BIGINT NOT NULL,
			position INT NOT NULL,
			PRIMARY KEY (post_id, media_id)
		)`,
	})
	return singleConnDB(db), cleanup
}

// 添付は入力が何件でも「media 行1本 + 紐付け1本」で保存される
// （以前は1件ごとに2往復していたので、4枚で8往復だった）。
// ID の採番と position の並びが1件ずつ保存していたときと同じであることも確かめる。
func TestCreateMediaBatch_TwoStatementsRegardlessOfCount(t *testing.T) {
	db, cleanup := mediaTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewMySQLMediaRepository(db)

	inputs := make([]model.MediaInput, 0, 4)
	for _, key := range []string{"a", "b", "c", "d"} {
		inputs = append(inputs, model.MediaInput{StorageKey: key, ContentType: "image/png"})
	}
	medias := model.NewMediaBatch(9, inputs, time.Unix(1700000000, 0))

	var err error
	counts := countStatements(t, db, func() {
		if err = repo.CreateMediaBatch(ctx, medias); err != nil {
			return
		}
		err = repo.CreatePostMediaBatch(ctx, 5, model.MediaIDs(medias), 0)
	})
	if err != nil {
		t.Fatalf("saving the attachments failed: %v", err)
	}
	if counts.inserts != 2 {
		t.Fatalf("expected 4 attachments to be saved with 2 INSERTs, got %d", counts.inserts)
	}

	// 採番されたIDが呼び出し側に戻っている（紐付けに使われている）。
	for i, m := range medias {
		if m.ID == 0 {
			t.Fatalf("media[%d] did not get an id back", i)
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT m.storage_key, pm.position FROM post_media pm JOIN media m ON m.id = pm.media_id WHERE pm.post_id = 5 ORDER BY pm.position`)
	if err != nil {
		t.Fatalf("failed to read back the attachments: %v", err)
	}
	defer rows.Close()

	var gotKeys []string
	pos := 0
	for rows.Next() {
		var key string
		var position int
		if err := rows.Scan(&key, &position); err != nil {
			t.Fatalf("failed to scan: %v", err)
		}
		if position != pos {
			t.Fatalf("expected position %d, got %d", pos, position)
		}
		pos++
		gotKeys = append(gotKeys, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("row iteration failed: %v", err)
	}
	if len(gotKeys) != 4 {
		t.Fatalf("expected 4 attachments, got %d", len(gotKeys))
	}
	for i, want := range []string{"a", "b", "c", "d"} {
		if gotKeys[i] != want {
			t.Fatalf("attachment %d: got %q, want %q (order must match the input)", i, gotKeys[i], want)
		}
	}

	// 編集で足すぶんは続きの番号から始まる。
	more := model.NewMediaBatch(9, []model.MediaInput{{StorageKey: "e", ContentType: "image/png"}}, time.Unix(1700000000, 0))
	if err := repo.CreateMediaBatch(ctx, more); err != nil {
		t.Fatalf("CreateMediaBatch for the edit failed: %v", err)
	}
	if err := repo.CreatePostMediaBatch(ctx, 5, model.MediaIDs(more), 4); err != nil {
		t.Fatalf("CreatePostMediaBatch for the edit failed: %v", err)
	}
	var position int
	if err := db.QueryRowContext(ctx, `SELECT position FROM post_media pm JOIN media m ON m.id = pm.media_id WHERE m.storage_key = 'e'`).Scan(&position); err != nil {
		t.Fatalf("failed to read the appended attachment: %v", err)
	}
	if position != 4 {
		t.Fatalf("expected the appended attachment at position 4, got %d", position)
	}
}

func timetableTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, cleanup := throwawaySchemaDB(t, "space_timetable_test", []string{
		`CREATE TABLE courses (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL DEFAULT 0,
			day_of_week VARCHAR(4) NOT NULL,
			period INT NOT NULL,
			teacher_name VARCHAR(255) NOT NULL DEFAULT '',
			course_name VARCHAR(255) NOT NULL DEFAULT '',
			year INT NOT NULL,
			semester VARCHAR(20) NOT NULL,
			dedup_key VARCHAR(255) NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE timetables (
			id BIGINT NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			course_id BIGINT NOT NULL,
			color VARCHAR(20) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
	})
	return singleConnDB(db), cleanup
}

// 学期まるごとの置き換えは、消す科目・足す科目が何件あっても DELETE 1本と INSERT 1本
// （以前は科目数ぶん往復していた）。結果の登録内容は1件ずつやっていたときと同じ。
func TestReplaceForSemester_OneDeleteAndOneInsert(t *testing.T) {
	db, cleanup := timetableTestDB(t)
	defer cleanup()

	ctx := context.Background()
	days := []string{"月", "火", "水", "木", "金", "土"}
	for i := 0; i < 6; i++ {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO courses (day_of_week, period, year, semester) VALUES (?, ?, 2026, '前期')`,
			days[i], i+1); err != nil {
			t.Fatalf("failed to seed a course: %v", err)
		}
	}

	repo := NewMySQLTimetableRepository(db)

	// 最初の登録: 1,2,3 を入れる（既存なし）。
	if _, err := repo.ReplaceForSemester(ctx, 1, 2026, "前期", nil, []int64{1, 2, 3}); err != nil {
		t.Fatalf("initial ReplaceForSemester failed: %v", err)
	}

	baseline, err := currentEntryIDs(ctx, db)
	if err != nil {
		t.Fatalf("failed to read the baseline: %v", err)
	}
	if len(baseline) != 3 {
		t.Fatalf("expected 3 entries after the initial registration, got %d", len(baseline))
	}

	// 1,2,3 を外して 4,5,6 を入れる = 3件削除・3件追加。
	var replaceErr error
	counts := countStatements(t, db, func() {
		_, replaceErr = repo.ReplaceForSemester(ctx, 1, 2026, "前期", baseline, []int64{4, 5, 6})
	})
	if replaceErr != nil {
		t.Fatalf("ReplaceForSemester failed: %v", replaceErr)
	}
	if counts.deletes != 1 {
		t.Fatalf("expected 3 removals to go out as 1 DELETE, got %d", counts.deletes)
	}
	if counts.inserts != 1 {
		t.Fatalf("expected 3 additions to go out as 1 INSERT, got %d", counts.inserts)
	}

	after, err := currentCourseIDs(ctx, db)
	if err != nil {
		t.Fatalf("failed to read the result: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("expected 3 entries after the replacement, got %d", len(after))
	}
	for _, courseID := range []int64{4, 5, 6} {
		if !after[courseID] {
			t.Fatalf("course %d should be registered after the replacement", courseID)
		}
	}
	for _, courseID := range []int64{1, 2, 3} {
		if after[courseID] {
			t.Fatalf("course %d should have been removed", courseID)
		}
	}
}

func currentEntryIDs(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM timetables ORDER BY id`)
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

func currentCourseIDs(ctx context.Context, db *sql.DB) (map[int64]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT course_id FROM timetables`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func pollTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, cleanup := throwawaySchemaDB(t, "space_poll_test", []string{
		`CREATE TABLE polls (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			author_user_id BIGINT NOT NULL,
			author_role VARCHAR(20) NOT NULL DEFAULT '',
			question TEXT NOT NULL,
			allow_multiple_choice BOOLEAN NOT NULL DEFAULT FALSE,
			deadline BIGINT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE poll_options (
			id BIGINT NOT NULL AUTO_INCREMENT,
			poll_id BIGINT NOT NULL,
			label VARCHAR(255) NOT NULL,
			display_order INT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE poll_votes (
			id BIGINT NOT NULL AUTO_INCREMENT,
			poll_option_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_poll_votes_option_user (poll_option_id, user_id)
		)`,
	})
	return singleConnDB(db), cleanup
}

// 投票の作成は選択肢が何件でも「polls 1本 + poll_options 1本」、
// 投票のやり直しは「DELETE 1本 + INSERT ... SELECT 1本」。
// どちらも以前は選択肢の数ぶん往復していた。
func TestPollWrites_AreBatched(t *testing.T) {
	db, cleanup := pollTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewMySQLPollRepository(db)

	labels := []string{"A", "B", "C", "D", "E"}
	var poll *model.Poll
	var err error
	counts := countStatements(t, db, func() {
		poll, err = repo.CreatePoll(ctx, repository.CreatePollParam{
			RoomID:              1,
			AuthorUserID:        2,
			AuthorRole:          "member",
			Question:            "どれ?",
			AllowMultipleChoice: true,
			OptionLabels:        labels,
		})
	})
	if err != nil {
		t.Fatalf("CreatePoll failed: %v", err)
	}
	if counts.inserts != 2 {
		t.Fatalf("expected the poll and its 5 options to go out as 2 INSERTs, got %d", counts.inserts)
	}

	// 並び順は渡した順のまま。
	options, err := repo.ListOptionsWithResults(ctx, poll.ID, 2)
	if err != nil {
		t.Fatalf("ListOptionsWithResults failed: %v", err)
	}
	if len(options) != len(labels) {
		t.Fatalf("expected %d options, got %d", len(labels), len(options))
	}
	for i, want := range labels {
		if options[i].Option.Label != want {
			t.Fatalf("option %d: got %q, want %q", i, options[i].Option.Label, want)
		}
		if options[i].Option.DisplayOrder != i {
			t.Fatalf("option %d: display_order is %d", i, options[i].Option.DisplayOrder)
		}
	}

	optionIDs := []int64{options[0].Option.ID, options[2].Option.ID, options[4].Option.ID}
	var voteErr error
	counts = countStatements(t, db, func() {
		voteErr = repo.ReplaceVotes(ctx, poll.ID, 7, optionIDs)
	})
	if voteErr != nil {
		t.Fatalf("ReplaceVotes failed: %v", voteErr)
	}
	if counts.inserts != 1 {
		t.Fatalf("expected 3 votes to go out as 1 INSERT, got %d", counts.inserts)
	}
	if counts.deletes != 1 {
		t.Fatalf("expected 1 DELETE for the previous votes, got %d", counts.deletes)
	}

	var votes int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM poll_votes WHERE user_id = 7`).Scan(&votes); err != nil {
		t.Fatalf("failed to count votes: %v", err)
	}
	if votes != 3 {
		t.Fatalf("expected 3 votes, got %d", votes)
	}

	// 他の投票の選択肢IDを混ぜても行は作られない（INSERT ... SELECT の
	// poll_id 条件が守り。1件ずつ撃っていたときと同じ守りが残っていること）。
	other, err := repo.CreatePoll(ctx, repository.CreatePollParam{
		RoomID: 1, AuthorUserID: 2, AuthorRole: "member", Question: "別の投票", OptionLabels: []string{"X"},
	})
	if err != nil {
		t.Fatalf("CreatePoll for the second poll failed: %v", err)
	}
	otherOptions, err := repo.ListOptionsWithResults(ctx, other.ID, 2)
	if err != nil {
		t.Fatalf("ListOptionsWithResults for the second poll failed: %v", err)
	}
	if err := repo.ReplaceVotes(ctx, poll.ID, 8, []int64{options[1].Option.ID, otherOptions[0].Option.ID}); err != nil {
		t.Fatalf("ReplaceVotes with a foreign option failed: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM poll_votes WHERE user_id = 8`).Scan(&votes); err != nil {
		t.Fatalf("failed to count votes: %v", err)
	}
	if votes != 1 {
		t.Fatalf("an option from another poll must not be votable; expected 1 vote, got %d", votes)
	}
}
