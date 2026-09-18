package mysql

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
)

// 「最後の管理者は削除できません」は COUNT と DELETE の隙間で壊れる種類の守りなので、
// 実際に2本の接続を同時に走らせないと確かめられない（Go 側の fake では
// トランザクションもロックも再現できない）。そのため MySQL を使う統合テストにしてある。
func administratorTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	return throwawaySchemaDB(t, "space_admin_test", []string{
		`CREATE TABLE administrators (
			id BIGINT NOT NULL AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			hashed_password VARCHAR(255) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
	})
}

// 管理者2人を同時に削除しても、片方は必ず残ること。
//
// 以前は COUNT と DELETE が別のトランザクションだったため、両方が COUNT=2 を
// 読んでから両方が消せて、管理者が0人になった（管理画面に誰も入れなくなる）。
func TestDeleteAdministrator_ConcurrentDeletesKeepTheLastOne(t *testing.T) {
	db, cleanup := administratorTestDB(t)
	defer cleanup()

	ctx := context.Background()
	for _, name := range []string{"admin-a", "admin-b"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO administrators (name, email, hashed_password, created_at, updated_at) VALUES (?, ?, 'x', 0, 0)`,
			name, name+"@example.com",
		); err != nil {
			t.Fatalf("failed to seed an administrator: %v", err)
		}
	}

	var ids []int64
	rows, err := db.QueryContext(ctx, `SELECT id FROM administrators ORDER BY id`)
	if err != nil {
		t.Fatalf("failed to read the seeded ids: %v", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("failed to scan an id: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatalf("expected 2 seeded administrators, got %d", len(ids))
	}

	uc := administrator.NewDeleteAdministratorUseCase(
		NewMySQLAdministratorRepository(db),
		NewMySQLTxManager(db),
	)

	// 両方の goroutine を同じ地点から走らせて、COUNT が重なる確率を上げる。
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	results := make([]bool, len(ids))
	errs := make([]error, len(ids))
	for i, id := range ids {
		done.Add(1)
		go func(i int, id int64) {
			defer done.Done()
			start.Wait()
			results[i], errs[i] = uc.Execute(context.Background(), id)
		}(i, id)
	}
	start.Done()
	done.Wait()

	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM administrators`).Scan(&remaining); err != nil {
		t.Fatalf("failed to count the remaining administrators: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining administrators = %d, want 1 (concurrent deletes must not remove every administrator)", remaining)
	}

	// ちょうど片方だけが成功していること。もう片方は文言つきで断られる。
	var succeeded int
	for i := range ids {
		if errs[i] == nil && results[i] {
			succeeded++
			continue
		}
		if errs[i] == nil {
			t.Fatalf("delete %d reported no error but did not delete anything", i)
		}
		if errs[i].Error() != "最後の管理者は削除できません" {
			t.Fatalf("delete %d failed with an unexpected error: %v", i, errs[i])
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful deletes = %d, want exactly 1", succeeded)
	}
}

// 1人しか居ないときは従来どおり削除できないこと（実 DB でも同じ文言で断ること）。
func TestDeleteAdministrator_RefusesToDeleteTheOnlyAdministrator(t *testing.T) {
	db, cleanup := administratorTestDB(t)
	defer cleanup()

	ctx := context.Background()
	res, err := db.ExecContext(ctx,
		`INSERT INTO administrators (name, email, hashed_password, created_at, updated_at) VALUES ('solo', 'solo@example.com', 'x', 0, 0)`)
	if err != nil {
		t.Fatalf("failed to seed an administrator: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to read the seeded id: %v", err)
	}

	uc := administrator.NewDeleteAdministratorUseCase(
		NewMySQLAdministratorRepository(db),
		NewMySQLTxManager(db),
	)

	ok, err := uc.Execute(ctx, id)
	if err == nil {
		t.Fatal("expected the last administrator to be protected")
	}
	if err.Error() != "最後の管理者は削除できません" {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}

	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM administrators`).Scan(&remaining); err != nil {
		t.Fatalf("failed to count the remaining administrators: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining administrators = %d, want 1", remaining)
	}
}
