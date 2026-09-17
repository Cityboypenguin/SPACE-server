package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// 匿名IDの採番は「同じ部屋で同じ番号を2人に渡さない」ことが要で、それを保証して
// いるのは Go 側のロジックではなく MySQL の1文（room_anonymous_sequences への
// INSERT ... ON DUPLICATE KEY UPDATE next_seq = LAST_INSERT_ID(next_seq) + 1）。
// つまり実際に MySQL へ並行に投げてみないと確かめられないので、ここは DB を使う
// 統合テストにしてある。
//
// 常用の go test ./... で MySQL を要求したくないので、DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/ -run Anonymous
//
// スキーマ名は末尾に付けない（テストが使い捨てスキーマを自分で作る）。開発用の
// space スキーマには一切触らない。
func anonymousTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed sequence test")
	}

	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open MySQL: %v", err)
	}
	if err := root.Ping(); err != nil {
		root.Close()
		t.Fatalf("failed to reach MySQL: %v", err)
	}

	schema := fmt.Sprintf("space_anon_test_%d", os.Getpid())
	if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to drop the throwaway schema: %v", err)
	}
	if _, err := root.Exec("CREATE DATABASE " + schema); err != nil {
		root.Close()
		t.Fatalf("failed to create the throwaway schema: %v", err)
	}

	db, err := sql.Open("mysql", dsn+schema)
	if err != nil {
		root.Close()
		t.Fatalf("failed to open the throwaway schema: %v", err)
	}

	// rooms / users は作らないので、db/migrations の DDL から外部キーだけ落とした形。
	// 見たいのは採番の競合で、参照整合性はここの関心ではない。
	ddl := []string{
		`CREATE TABLE room_anonymous_identities (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			label VARCHAR(50) NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY unique_room_anon_room_user (room_id, user_id),
			UNIQUE KEY unique_room_anon_room_label (room_id, label)
		)`,
		`CREATE TABLE room_anonymous_sequences (
			room_id BIGINT NOT NULL,
			next_seq BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (room_id)
		)`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			root.Close()
			t.Fatalf("failed to create a test table: %v", err)
		}
	}

	return db, func() {
		db.Close()
		if _, err := root.Exec("DROP DATABASE IF EXISTS " + schema); err != nil {
			t.Errorf("failed to clean up the throwaway schema %s: %v", schema, err)
		}
		root.Close()
	}
}

func TestGetOrCreateAnonymousIdentity_ConcurrentAllocationsAreUnique(t *testing.T) {
	db, cleanup := anonymousTestDB(t)
	defer cleanup()

	// 採番が別接続へ振り分けられる状況を作る（旧実装の GET_LOCK はここで壊れていた）。
	db.SetMaxOpenConns(10)

	repo := NewMySQLRoomAnonymousIdentityRepository(db)
	const roomID int64 = 1
	const users = 30

	var wg sync.WaitGroup
	labels := make([]string, users)
	errs := make([]error, users)
	start := make(chan struct{})
	for i := 0; i < users; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			identity, err := repo.GetOrCreate(context.Background(), roomID, int64(i+1))
			if err != nil {
				errs[i] = err
				return
			}
			labels[i] = identity.Label
		}(i)
	}
	close(start)
	wg.Wait()

	seen := make(map[string]int, users)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("user %d failed to get an identity: %v", i+1, err)
		}
		if prev, dup := seen[labels[i]]; dup {
			t.Fatalf("label %q was handed to both user %d and user %d", labels[i], prev, i+1)
		}
		seen[labels[i]] = i + 1
	}
	if len(seen) != users {
		t.Fatalf("distinct labels = %d, want %d", len(seen), users)
	}
}

func TestGetOrCreateAnonymousIdentity_NumbersAreNotReusedAfterDeletion(t *testing.T) {
	db, cleanup := anonymousTestDB(t)
	defer cleanup()

	repo := NewMySQLRoomAnonymousIdentityRepository(db)
	ctx := context.Background()
	const roomID int64 = 2

	first, err := repo.GetOrCreate(ctx, roomID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Label != "匿名001" {
		t.Fatalf("first label = %q, want 匿名001", first.Label)
	}
	second, err := repo.GetOrCreate(ctx, roomID, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.Label != "匿名002" {
		t.Fatalf("second label = %q, want 匿名002", second.Label)
	}

	// 退会などで途中の行が消えた状況（旧実装はここで COUNT が戻り、匿名002 を再発行した）。
	if _, err := db.ExecContext(ctx, `DELETE FROM room_anonymous_identities WHERE room_id = ? AND user_id = ?`, roomID, int64(1)); err != nil {
		t.Fatalf("failed to delete a row: %v", err)
	}

	third, err := repo.GetOrCreate(ctx, roomID, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if third.Label != "匿名003" {
		t.Fatalf("label after a deletion = %q, want 匿名003 (番号は再利用しない)", third.Label)
	}
}

func TestGetOrCreateAnonymousIdentity_IsStableForTheSameUser(t *testing.T) {
	db, cleanup := anonymousTestDB(t)
	defer cleanup()

	repo := NewMySQLRoomAnonymousIdentityRepository(db)
	ctx := context.Background()
	const roomID int64 = 3

	first, err := repo.GetOrCreate(ctx, roomID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	again, err := repo.GetOrCreate(ctx, roomID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if again.Label != first.Label || again.ID != first.ID {
		t.Fatalf("identity changed between calls: %+v then %+v (F-05: 匿名IDは授業ごとに固定)", first, again)
	}
}
