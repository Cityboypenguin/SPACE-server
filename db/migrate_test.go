package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// マイグレーションを実物の MySQL に当てるテスト。
//
// 外部キーの張り替えは制約名を名指しする。名前の無い外部キーは MySQL が
// 「表名_ibfk_N」と自動で付けるので、定義の順番を読み違えると本番の起動時に
// マイグレーションが落ち、golang-migrate が dirty のまま止まる。Go 側のモックでは
// 確かめようが無いので、実物で全部を流す。
//
// 常用の go test ./... で MySQL を要求したくないので、DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./db/
//
// スキーマ名は末尾に付けない（テストが使い捨てスキーマを自分で作る）。

// beforeAccountSplit は users を識別子と個人情報に分ける直前の版。
const beforeAccountSplit = 75

func throwawayDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed migration test")
	}
	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { root.Close() })

	schema := fmt.Sprintf("space_migration_test_%d", os.Getpid())
	if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		t.Fatalf("drop the throwaway schema: %v", err)
	}
	if _, err := root.Exec("CREATE DATABASE " + schema); err != nil {
		t.Fatalf("create the throwaway schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
			t.Errorf("clean up the throwaway schema %s: %v", schema, err)
		}
	})

	db, err := sql.Open("mysql", dsn+schema)
	if err != nil {
		t.Fatalf("open the throwaway schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newMigrator(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		t.Fatalf("open migrations: %v", err)
	}
	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		t.Fatalf("migration driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	return m
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

// fkRule は「この列はどの表を、どの削除規則で参照しているか」。
type fkRule struct{ target, rule string }

func foreignKeysToUsers(t *testing.T, db *sql.DB) map[string]fkRule {
	t.Helper()
	rows, err := db.Query(`
		SELECT k.TABLE_NAME, k.COLUMN_NAME, k.REFERENCED_TABLE_NAME, r.DELETE_RULE
		FROM information_schema.KEY_COLUMN_USAGE k
		JOIN information_schema.REFERENTIAL_CONSTRAINTS r
		  ON r.CONSTRAINT_SCHEMA = k.TABLE_SCHEMA AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME AND r.TABLE_NAME = k.TABLE_NAME
		WHERE k.TABLE_SCHEMA = DATABASE() AND k.REFERENCED_TABLE_NAME IN ('users', 'user_accounts')`)
	if err != nil {
		t.Fatalf("list foreign keys: %v", err)
	}
	defer rows.Close()
	out := map[string]fkRule{}
	for rows.Next() {
		var table, column, target, rule string
		if err := rows.Scan(&table, &column, &target, &rule); err != nil {
			t.Fatalf("scan foreign key: %v", err)
		}
		out[table+"."+column] = fkRule{target, rule}
	}
	return out
}

func columns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? ORDER BY COLUMN_NAME`, table)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		out = append(out, c)
	}
	return out
}

// seedBeforeSplit は分割前の形で、利用者2人とそれぞれの持ち物・会話を入れる。
func seedBeforeSplit(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO users (id, account_id, name, email, hashed_password, role, status, created_at, updated_at, last_active_at, credentials_version)
		VALUES (1, 'taro', '山田太郎', 'taro@example.com', 'hash-1', 'student', 'active', 100, 110, 120, 3),
		       (2, 'hana', '佐藤花子', 'hana@example.com', 'hash-2', 'teacher', 'frozen', 200, 210, NULL, 0)`)
	mustExec(t, db, `INSERT INTO rooms (id, name, type, created_at, updated_at) VALUES (10, 'DM', 'dm', 1, 1)`)
	mustExec(t, db, `INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (10, 1, 'member', 1, 1), (10, 2, 'member', 1, 1)`)
	mustExec(t, db, `INSERT INTO messages (id, room_id, user_id, content, created_at, updated_at) VALUES (100, 10, 1, 'x', 1, 1)`)
	mustExec(t, db, `INSERT INTO posts (id, user_id, content, created_at, updated_at) VALUES (200, 1, 'hello', 1, 1)`)
	mustExec(t, db, `INSERT INTO profiles (user_id, bio, created_at, updated_at) VALUES (1, '', 1, 1)`)
}

func TestAccountSplitMigration(t *testing.T) {
	db := throwawayDB(t)
	m := newMigrator(t, db)

	if err := m.Migrate(beforeAccountSplit); err != nil {
		t.Fatalf("migrate to %d: %v", beforeAccountSplit, err)
	}
	seedBeforeSplit(t, db)

	if err := m.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	t.Run("users keeps only the identity", func(t *testing.T) {
		got := strings.Join(columns(t, db, "users"), ",")
		if want := "created_at,deactivated_at,deleted_at,id,status"; got != want {
			t.Fatalf("users columns = %s, want %s", got, want)
		}
	})

	t.Run("personal data moved to user_accounts", func(t *testing.T) {
		var accountID, name, email, hash, role string
		var version, updatedAt int64
		var lastActive sql.NullInt64
		err := db.QueryRow(`SELECT account_id, name, email, hashed_password, role, credentials_version, last_active_at, updated_at FROM user_accounts WHERE user_id = 1`).
			Scan(&accountID, &name, &email, &hash, &role, &version, &lastActive, &updatedAt)
		if err != nil {
			t.Fatalf("read user_accounts: %v", err)
		}
		if accountID != "taro" || name != "山田太郎" || email != "taro@example.com" || hash != "hash-1" ||
			role != "student" || version != 3 || lastActive.Int64 != 120 || updatedAt != 110 {
			t.Fatalf("user 1 = %s %s %s %s %s %d %v %d", accountID, name, email, hash, role, version, lastActive, updatedAt)
		}
		var status string
		var createdAt int64
		if err := db.QueryRow(`SELECT status, created_at FROM users WHERE id = 2`).Scan(&status, &createdAt); err != nil {
			t.Fatalf("read users: %v", err)
		}
		if status != "frozen" || createdAt != 200 {
			t.Fatalf("user 2 = %s %d, want frozen 200", status, createdAt)
		}
		if n := count(t, db, `SELECT COUNT(*) FROM user_accounts`); n != 2 {
			t.Fatalf("user_accounts rows = %d, want 2", n)
		}
	})

	t.Run("foreign keys point where their rows belong", func(t *testing.T) {
		fks := foreignKeysToUsers(t, db)
		kept := []string{
			"messages.user_id", "questions.asker_user_id", "answers.author_user_id",
			"polls.author_user_id", "poll_votes.user_id", "answer_likes.user_id",
		}
		for _, col := range kept {
			if got := fks[col]; got != (fkRule{"users", "RESTRICT"}) {
				t.Errorf("%s -> %+v, want users RESTRICT (退会後も残す)", col, got)
			}
			delete(fks, col)
		}
		if got := fks["user_accounts.user_id"]; got != (fkRule{"users", "RESTRICT"}) {
			t.Errorf("user_accounts.user_id -> %+v, want users RESTRICT", got)
		}
		delete(fks, "user_accounts.user_id")

		// 残りは全部、個人情報と一緒に消える持ち物。
		var stray []string
		for col, got := range fks {
			if got != (fkRule{"user_accounts", "CASCADE"}) {
				stray = append(stray, fmt.Sprintf("%s -> %+v", col, got))
			}
		}
		sort.Strings(stray)
		if len(stray) > 0 {
			t.Errorf("want user_accounts CASCADE, got:\n%s", strings.Join(stray, "\n"))
		}
		if len(fks) != 18 {
			t.Errorf("account-bound foreign keys = %d, want 18", len(fks))
		}
	})

	t.Run("purging the account keeps the conversation", func(t *testing.T) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM user_accounts WHERE user_id = 1`); err != nil {
			t.Fatalf("delete the account: %v", err)
		}
		for _, q := range []string{
			`SELECT COUNT(*) FROM posts WHERE user_id = 1`,
			`SELECT COUNT(*) FROM profiles WHERE user_id = 1`,
			`SELECT COUNT(*) FROM room_users WHERE user_id = 1`,
		} {
			var n int
			if err := tx.QueryRow(q).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Errorf("%s = %d, want 0 (個人情報と一緒に消えるはず)", q, n)
			}
		}
		var messages int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM messages WHERE user_id = 1`).Scan(&messages); err != nil {
			t.Fatal(err)
		}
		if messages != 1 {
			t.Errorf("messages = %d, want 1 (会話は残すはず)", messages)
		}
		_, err = tx.Exec(`DELETE FROM users WHERE id = 1`)
		if err == nil || !strings.Contains(err.Error(), "foreign key constraint") {
			t.Errorf("deleting the identity row = %v, want a foreign key error (RESTRICT)", err)
		}
	})

	t.Run("down restores the original shape", func(t *testing.T) {
		if err := m.Migrate(beforeAccountSplit); err != nil {
			t.Fatalf("migrate down to %d: %v", beforeAccountSplit, err)
		}
		var accountID, email, hash string
		var version int64
		if err := db.QueryRow(`SELECT account_id, email, hashed_password, credentials_version FROM users WHERE id = 1`).
			Scan(&accountID, &email, &hash, &version); err != nil {
			t.Fatalf("read users after down: %v", err)
		}
		if accountID != "taro" || email != "taro@example.com" || hash != "hash-1" || version != 3 {
			t.Fatalf("user 1 after down = %s %s %s %d", accountID, email, hash, version)
		}
		for col, got := range foreignKeysToUsers(t, db) {
			if got != (fkRule{"users", "CASCADE"}) {
				t.Errorf("%s -> %+v after down, want users CASCADE", col, got)
			}
		}
		// 戻した後にもう一度上げられること（down が途中の状態を残していない）。
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			t.Fatalf("migrate up again: %v", err)
		}
	})
}

// beforeCourseSync はシラバス同期の列を足す直前の版。
const beforeCourseSync = 102

// シラバス同期の migration（103〜108）が既存の授業を1行も失わず、
// 取り込み元と講義コードを dedup_key から正しく埋めること。
// 戻したときも行が残ること（down は列を落とすだけ）。
func TestCourseSyncMigration(t *testing.T) {
	db := throwawayDB(t)
	m := newMigrator(t, db)

	if err := m.Migrate(beforeCourseSync); err != nil {
		t.Fatalf("migrate to %d: %v", beforeCourseSync, err)
	}
	mustExec(t, db, `INSERT INTO rooms (id, name, type, created_at, updated_at) VALUES
		(1, '経済学入門', 'course', 1, 1), (2, '手作りの授業', 'course', 1, 1), (3, '古い形式', 'course', 1, 1)`)
	mustExec(t, db, `INSERT INTO courses (id, room_id, day_of_week, period, teacher_name, course_name, year, semester, dedup_key, created_at, updated_at) VALUES
		(1, 1, '水', 5, '田中', '経済学入門', 2026, '前期', 'senshu:2026:前期:12345:水:5', 1, 1),
		(2, 2, '月', 1, '佐藤', '手作りの授業', 2026, '前期', 'manual:0b9c0e4e-0000-0000-0000-000000000000', 1, 1),
		(3, 3, '火', 2, '鈴木', '古い形式', 2026, '前期', 'legacy-key', 1, 1)`)

	if err := m.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	if n := count(t, db, `SELECT COUNT(*) FROM courses`); n != 3 {
		t.Fatalf("courses = %d, want 3 (no row may be lost)", n)
	}
	type row struct {
		source    string
		sourceRef sql.NullString
		dedupKey  string
		disc      sql.NullInt64
	}
	read := func(id int) row {
		var r row
		if err := db.QueryRow(`SELECT source, source_ref, dedup_key, discontinued_at FROM courses WHERE id = ?`, id).
			Scan(&r.source, &r.sourceRef, &r.dedupKey, &r.disc); err != nil {
			t.Fatalf("read course %d: %v", id, err)
		}
		return r
	}
	if r := read(1); r.source != "senshu" || r.sourceRef.String != "12345" || r.dedupKey != "senshu:2026:前期:12345:水:5" || r.disc.Valid {
		t.Errorf("scraped course = %+v, want senshu / 12345 / dedup_key kept / not discontinued", r)
	}
	if r := read(2); r.source != "manual" || r.sourceRef.Valid {
		t.Errorf("manual course = %+v, want manual with no source_ref", r)
	}
	if r := read(3); r.source != "unknown" || r.sourceRef.Valid {
		t.Errorf("odd course = %+v, want unknown with no source_ref (never synced)", r)
	}
	for _, table := range []string{"course_sync_reviews", "course_sync_runs", "course_sync_changes"} {
		if n := count(t, db, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table); n != 1 {
			t.Errorf("table %s was not created", table)
		}
	}

	if err := m.Migrate(beforeCourseSync); err != nil {
		t.Fatalf("migrate back down to %d: %v", beforeCourseSync, err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM courses`); n != 3 {
		t.Fatalf("courses after down = %d, want 3", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'courses' AND COLUMN_NAME = 'source'`); n != 0 {
		t.Errorf("courses.source survived the down migration")
	}
}
