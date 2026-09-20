package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

func activityArchiveTestDB(t *testing.T) (*MySQLActivityArchiveRepository, func()) {
	t.Helper()
	db, cleanup := throwawaySchemaDB(t, "space_activity_archive_test", []string{
		`CREATE TABLE user_activity_hours (
			user_id BIGINT NOT NULL,
			activity_hour DATETIME NOT NULL,
			PRIMARY KEY (user_id, activity_hour),
			INDEX idx_activity_hour (activity_hour)
		) ENGINE=InnoDB`,
		`CREATE TABLE user_activity_archives (
			archive_month DATE NOT NULL,
			object_key VARCHAR(512) NOT NULL,
			row_count BIGINT NOT NULL,
			sha256 CHAR(64) NOT NULL,
			archived_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			PRIMARY KEY (archive_month),
			INDEX idx_user_activity_archives_expires_at (expires_at)
		) ENGINE=InnoDB`,
	})
	return NewMySQLActivityArchiveRepository(db), cleanup
}

func TestActivityArchiveUsesJSTWallClockMonthBoundaries(t *testing.T) {
	repo, cleanup := activityArchiveTestDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO user_activity_hours (user_id, activity_hour) VALUES
		(1, '2025-01-31 20:00:00'), (2, '2025-02-01 01:00:00')`); err != nil {
		t.Fatal(err)
	}

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, activityArchiveJST)
	to := from.AddDate(0, 1, 0)
	var got []repository.ActivityHour
	count, err := repo.StreamActivityHours(ctx, from, to, func(row repository.ActivityHour) error {
		got = append(got, row)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(got) != 1 || got[0].UserID != 1 || got[0].ActivityHour.Hour() != 20 || got[0].ActivityHour.Location() != activityArchiveJST {
		t.Fatalf("January rows = %#v (count=%d)", got, count)
	}

	oldest, err := repo.OldestActivityHourBefore(ctx, time.Date(2025, 2, 2, 0, 0, 0, 0, activityArchiveJST))
	if err != nil {
		t.Fatal(err)
	}
	if oldest == nil || oldest.Month() != time.January || oldest.Day() != 31 || oldest.Hour() != 20 {
		t.Fatalf("oldest = %v", oldest)
	}
}

func TestFinalizeActivityArchiveRollsBackOnRowCountMismatch(t *testing.T) {
	repo, cleanup := activityArchiveTestDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO user_activity_hours (user_id, activity_hour) VALUES (1, '2025-01-10 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	month := time.Date(2025, 1, 1, 0, 0, 0, 0, activityArchiveJST)
	err := repo.FinalizeActivityArchive(ctx, repository.ActivityArchive{
		Month: month, ObjectKey: "archive", RowCount: 2, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ArchivedAt: month.AddDate(1, 0, 0), ExpiresAt: month.AddDate(4, 0, 0),
	})
	if err == nil {
		t.Fatal("row-count mismatch must fail")
	}
	var hours, ledger int
	if err := repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_activity_hours`).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_activity_archives`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if hours != 1 || ledger != 0 {
		t.Fatalf("rollback failed: hours=%d ledger=%d", hours, ledger)
	}

	archivedAt := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	expiresAt := archivedAt.AddDate(3, 0, 0)
	if err := repo.FinalizeActivityArchive(ctx, repository.ActivityArchive{
		Month: month, ObjectKey: "archive", RowCount: 1, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ArchivedAt: archivedAt, ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_activity_hours`).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_activity_archives`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if hours != 0 || ledger != 1 {
		t.Fatalf("finalize failed: hours=%d ledger=%d", hours, ledger)
	}
	for key, want := range map[string]bool{"archive": true, "unreferenced": false} {
		got, err := repo.IsActivityArchiveObjectReferenced(ctx, key)
		if err != nil || got != want {
			t.Fatalf("reference %q = %v, err=%v, want %v", key, got, err, want)
		}
	}
	expired, err := repo.ListExpiredActivityArchives(ctx, expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || !expired[0].Month.Equal(month) || expired[0].ObjectKey != "archive" {
		t.Fatalf("expired archives = %#v", expired)
	}
}

func TestActivityArchiveLockIsConnectionScopedAndExclusive(t *testing.T) {
	repo, cleanup := activityArchiveTestDB(t)
	defer cleanup()
	ctx := context.Background()
	release, acquired, err := repo.AcquireLock(ctx)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v err=%v", acquired, err)
	}
	_, acquired, err = repo.AcquireLock(ctx)
	if err != nil || acquired {
		t.Fatalf("second lock: acquired=%v err=%v", acquired, err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	release, acquired, err = repo.AcquireLock(ctx)
	if err != nil || !acquired {
		t.Fatalf("lock after release: acquired=%v err=%v", acquired, err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestMigration071RoundTrip(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_migration071_test", nil)
	defer cleanup()
	for _, name := range []string{"071_create_user_activity_archives.up.sql", "071_create_user_activity_archives.down.sql"} {
		body, err := os.ReadFile("../../db/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
