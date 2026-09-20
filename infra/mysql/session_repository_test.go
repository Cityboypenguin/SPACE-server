package mysql

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// countingExec は「何本 SQL を投げたか」だけを数える dbtx。
// バルク INSERT にまとめた効果は往復回数でしか見えないので、ここで数える。
type countingExec struct {
	execs    int
	lastArgs int
}

func (c *countingExec) ExecContext(_ context.Context, _ string, args ...any) (sql.Result, error) {
	c.execs++
	c.lastArgs = len(args)
	return driverResult{}, nil
}

func (c *countingExec) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("not used")
}

func (c *countingExec) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("not used")
}

type driverResult struct{}

func (driverResult) LastInsertId() (int64, error) { return 0, nil }
func (driverResult) RowsAffected() (int64, error) { return 0, nil }

func samplePageViews(n int) []repository.PageViewInput {
	out := make([]repository.PageViewInput, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, repository.PageViewInput{Path: "/p", DurationSeconds: 1, MaxScrollDepth: 2})
	}
	return out
}

// 1セッションぶんのページビューは、何画面ぶんあっても INSERT 1本にまとまる
// （以前は画面数ぶん ExecContext していた）。
func TestInsertPageViews_SendsOneStatement(t *testing.T) {
	db := &countingExec{}
	if err := insertPageViews(context.Background(), db, 1, "2026-01-01", 1700000000, samplePageViews(40)); err != nil {
		t.Fatalf("insertPageViews returned an error: %v", err)
	}
	if db.execs != 1 {
		t.Fatalf("expected 40 page views to go out as 1 statement, got %d", db.execs)
	}
	if want := 40 * pageViewStatColumns; db.lastArgs != want {
		t.Fatalf("expected %d placeholders, got %d", want, db.lastArgs)
	}
}

// 0件なら DB へ行かない。
func TestInsertPageViews_SkipsTheRoundTripWhenEmpty(t *testing.T) {
	db := &countingExec{}
	if err := insertPageViews(context.Background(), db, 1, "2026-01-01", 1700000000, nil); err != nil {
		t.Fatalf("insertPageViews returned an error: %v", err)
	}
	if db.execs != 0 {
		t.Fatalf("expected no statements for an empty batch, got %d", db.execs)
	}
}

// プレースホルダ上限を超える件数は分割する（まとめたせいで落ちる、を起こさない）。
func TestInsertPageViews_SplitsAboveThePlaceholderLimit(t *testing.T) {
	perStatement := bulkInsertChunkSize(pageViewStatColumns)
	db := &countingExec{}
	if err := insertPageViews(context.Background(), db, 1, "2026-01-01", 1700000000, samplePageViews(perStatement+1)); err != nil {
		t.Fatalf("insertPageViews returned an error: %v", err)
	}
	if db.execs != 2 {
		t.Fatalf("expected the batch to be split into 2 statements, got %d", db.execs)
	}
	if db.lastArgs > maxBulkInsertParams {
		t.Fatalf("a statement carried %d placeholders, over the %d limit", db.lastArgs, maxBulkInsertParams)
	}
}

func sessionTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	return throwawaySchemaDB(t, "space_session_test", []string{
		`CREATE TABLE user_session_summaries (
			id BIGINT NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			date VARCHAR(10) NOT NULL,
			session_count INT NOT NULL DEFAULT 0,
			total_duration_seconds INT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uk_user_date (user_id, date)
		)`,
		`CREATE TABLE page_view_stats (
			id BIGINT NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			date VARCHAR(10) NOT NULL,
			page_path VARCHAR(255) NOT NULL,
			view_count INT NOT NULL DEFAULT 0,
			total_duration_seconds INT NOT NULL DEFAULT 0,
			total_max_scroll_depth INT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uk_user_date_path (user_id, date, page_path)
		)`,
	})
}

// まとめた INSERT が、1件ずつ撃っていたときと同じ行・同じ積み上がり方になること。
// 同じ画面を複数回踏んだぶんも、ON DUPLICATE KEY UPDATE が1行に足し込む。
func TestRecordSession_BulkInsertMatchesOneAtATime(t *testing.T) {
	db, cleanup := sessionTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewMySQLSessionRepository(db)

	pageViews := []repository.PageViewInput{
		{Path: "/timeline", DurationSeconds: 30, MaxScrollDepth: 80},
		{Path: "/dm", DurationSeconds: 10, MaxScrollDepth: 20},
		{Path: "/timeline", DurationSeconds: 5, MaxScrollDepth: 10},
	}
	if err := repo.RecordSession(ctx, 1, 45, pageViews); err != nil {
		t.Fatalf("RecordSession returned an error: %v", err)
	}

	type row struct {
		views    int
		duration int
		scroll   int
	}
	got := map[string]row{}
	rows, err := db.QueryContext(ctx, `SELECT page_path, view_count, total_duration_seconds, total_max_scroll_depth FROM page_view_stats`)
	if err != nil {
		t.Fatalf("failed to read page_view_stats: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var r row
		if err := rows.Scan(&path, &r.views, &r.duration, &r.scroll); err != nil {
			t.Fatalf("failed to scan: %v", err)
		}
		got[path] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("row iteration failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 distinct page rows, got %d (%v)", len(got), got)
	}
	// 同じ画面を2回踏んだぶんは1行に足し込まれる（1件ずつ撃っていたときと同じ）。
	if want := (row{views: 2, duration: 35, scroll: 90}); got["/timeline"] != want {
		t.Fatalf("/timeline: got %+v, want %+v", got["/timeline"], want)
	}
	if want := (row{views: 1, duration: 10, scroll: 20}); got["/dm"] != want {
		t.Fatalf("/dm: got %+v, want %+v", got["/dm"], want)
	}

	// 2セッション目も同じ行へ積み上がる。
	if err := repo.RecordSession(ctx, 1, 15, []repository.PageViewInput{
		{Path: "/timeline", DurationSeconds: 7, MaxScrollDepth: 5},
	}); err != nil {
		t.Fatalf("second RecordSession returned an error: %v", err)
	}
	var views, duration, sessions int
	if err := db.QueryRowContext(ctx,
		`SELECT view_count, total_duration_seconds FROM page_view_stats WHERE page_path = '/timeline'`,
	).Scan(&views, &duration); err != nil {
		t.Fatalf("failed to re-read /timeline: %v", err)
	}
	if views != 3 || duration != 42 {
		t.Fatalf("expected /timeline to accumulate to 3 views / 42s, got %d / %d", views, duration)
	}
	if err := db.QueryRowContext(ctx, `SELECT session_count FROM user_session_summaries`).Scan(&sessions); err != nil {
		t.Fatalf("failed to read user_session_summaries: %v", err)
	}
	if sessions != 2 {
		t.Fatalf("expected 2 sessions recorded, got %d", sessions)
	}
}
