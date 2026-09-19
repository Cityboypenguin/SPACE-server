package mysql

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteUserClearsLiveActivityAndRejectsLateWrites(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_activity_erasure_test", []string{
		`CREATE TABLE users (id BIGINT NOT NULL PRIMARY KEY) ENGINE=InnoDB`,
		`CREATE TABLE user_activity_dates (user_id BIGINT NOT NULL, activity_date DATE NOT NULL, PRIMARY KEY (user_id, activity_date)) ENGINE=InnoDB`,
		`CREATE TABLE user_activity_hours (user_id BIGINT NOT NULL, activity_hour DATETIME NOT NULL, PRIMARY KEY (user_id, activity_hour)) ENGINE=InnoDB`,
	})
	defer cleanup()
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO users (id) VALUES (1), (2)`,
		`INSERT INTO user_activity_dates (user_id, activity_date) VALUES (1, '2026-09-18'), (2, '2026-09-18')`,
		`INSERT INTO user_activity_hours (user_id, activity_hour) VALUES (1, '2026-09-18 10:00:00'), (2, '2026-09-18 10:00:00')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	repo := NewMySQLUserRepository(db)
	txManager := NewMySQLTxManager(db)
	if err := txManager.RunInTx(ctx, func(ctx context.Context) error {
		deleted, err := repo.DeleteUser(ctx, 1)
		if err != nil || !deleted {
			t.Fatalf("DeleteUser: deleted=%v err=%v", deleted, err)
		}
		return repo.DeleteActivityHistory(ctx, 1)
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.LogActivityDate(ctx, 1, "2026-09-19"); err != nil {
		t.Fatal(err)
	}
	if err := repo.LogActivityHour(ctx, 1, "2026-09-19 10:00:00"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"user_activity_dates", "user_activity_hours"} {
		var deletedCount, remainingCount int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE user_id = 1").Scan(&deletedCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE user_id = 2").Scan(&remainingCount); err != nil {
			t.Fatal(err)
		}
		if deletedCount != 0 || remainingCount != 1 {
			t.Fatalf("%s: deleted=%d remaining=%d", table, deletedCount, remainingCount)
		}
	}

	rollback := errors.New("rollback")
	if err := txManager.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := repo.DeleteUser(ctx, 2); err != nil {
			return err
		}
		if err := repo.DeleteActivityHistory(ctx, 2); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}
	var users, hours int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = 2`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_activity_hours WHERE user_id = 2`).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if users != 1 || hours != 1 {
		t.Fatalf("rollback did not restore user and activity: users=%d hours=%d", users, hours)
	}
}
