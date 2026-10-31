package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// リアクションの集計 SQL（GROUP BY / ORDER BY MIN(id)）と、UNIQUE 制約・CASCADE の効き方は
// 実際に MySQL へ投げないと確かめられないので、DB を使う統合テストにしてある。
// 常用の go test ./... で MySQL を要求しないよう、DSN が渡されたときだけ走る。
//
//	SPACE_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:3306)/' go test ./infra/mysql/ -run MessageReaction
//
// スキーマは使い捨てで、開発用の space スキーマには触らない。テーブルは本物の
// マイグレーション（074）をそのまま流すので、参照先の users / messages だけ最小の形で作る。
func messageReactionTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	dsn := os.Getenv("SPACE_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SPACE_TEST_MYSQL_DSN is not set; skipping the MySQL-backed message reaction test")
	}

	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to open MySQL: %v", err)
	}
	if err := root.Ping(); err != nil {
		root.Close()
		t.Fatalf("failed to reach MySQL: %v", err)
	}

	schema := fmt.Sprintf("space_message_reaction_test_%d", os.Getpid())
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

	migration, err := os.ReadFile("../../db/migrations/074_create_message_reactions.up.sql")
	if err != nil {
		t.Fatalf("failed to read the migration: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE users (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))`,
		`CREATE TABLE messages (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))`,
		`INSERT INTO users (id) VALUES (1), (2), (3)`,
		`INSERT INTO messages (id) VALUES (10), (11), (12)`,
		string(migration),
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			root.Close()
			t.Fatalf("failed to prepare the schema: %v\n%s", err, stmt)
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

func TestMessageReaction_AddIsIdempotentAndCountsPerEmoji(t *testing.T) {
	db, cleanup := messageReactionTestDB(t)
	defer cleanup()

	repo := NewMySQLMessageReactionRepository(db)
	ctx := context.Background()

	// 同じ人が同じ絵文字を2回付けても1件。別の人が付ければ2件になる。
	for _, userID := range []int64{1, 1, 2} {
		if err := repo.AddMessageReaction(ctx, 10, userID, "👍"); err != nil {
			t.Fatalf("AddMessageReaction(user %d) failed: %v", userID, err)
		}
	}
	// 1人が別の絵文字を付けることもできる（絵文字ごとに別行）。
	if err := repo.AddMessageReaction(ctx, 10, 1, "🙏"); err != nil {
		t.Fatalf("AddMessageReaction(second emoji) failed: %v", err)
	}

	got, err := repo.ListMessageReactionsByMessageIDs(ctx, []int64{10, 11}, 1)
	if err != nil {
		t.Fatalf("ListMessageReactionsByMessageIDs failed: %v", err)
	}
	if _, ok := got[11]; ok {
		t.Error("a message with no reactions must not appear in the result")
	}
	reactions := got[10]
	if len(reactions) != 2 {
		t.Fatalf("got %d emoji groups, want 2: %+v", len(reactions), reactions)
	}
	// 付けられた順（先に付いた 👍 が前）。
	if reactions[0].Emoji != "👍" || reactions[0].Count != 2 || !reactions[0].ReactedByMe {
		t.Errorf("first group = %+v, want 👍 x2 reacted by me", reactions[0])
	}
	if reactions[1].Emoji != "🙏" || reactions[1].Count != 1 || !reactions[1].ReactedByMe {
		t.Errorf("second group = %+v, want 🙏 x1 reacted by me", reactions[1])
	}

	// 閲覧者が違えば reactedByMe だけが変わり、数は同じ。
	other, err := repo.ListMessageReactionsByMessageIDs(ctx, []int64{10}, 3)
	if err != nil {
		t.Fatalf("ListMessageReactionsByMessageIDs failed: %v", err)
	}
	if other[10][0].Count != 2 || other[10][0].ReactedByMe {
		t.Errorf("viewer 3 sees %+v, want the same count but not reacted by me", other[10][0])
	}
}

func TestMessageReaction_RemoveOnlyAffectsTheCallersOwnReaction(t *testing.T) {
	db, cleanup := messageReactionTestDB(t)
	defer cleanup()

	repo := NewMySQLMessageReactionRepository(db)
	ctx := context.Background()

	for _, userID := range []int64{1, 2} {
		if err := repo.AddMessageReaction(ctx, 10, userID, "👍"); err != nil {
			t.Fatalf("AddMessageReaction failed: %v", err)
		}
	}

	// 外す操作は冪等。付けていない人（3）が外しても、他の人の分は消えない。
	for _, userID := range []int64{1, 1, 3} {
		if err := repo.RemoveMessageReaction(ctx, 10, userID, "👍"); err != nil {
			t.Fatalf("RemoveMessageReaction(user %d) failed: %v", userID, err)
		}
	}

	got, err := repo.ListMessageReactionsByMessageIDs(ctx, []int64{10}, 2)
	if err != nil {
		t.Fatalf("ListMessageReactionsByMessageIDs failed: %v", err)
	}
	if len(got[10]) != 1 || got[10][0].Count != 1 || !got[10][0].ReactedByMe {
		t.Errorf("got %+v, want only user 2's reaction left", got[10])
	}
}

func TestMessageReaction_RowsAreRemovedWithTheirMessage(t *testing.T) {
	db, cleanup := messageReactionTestDB(t)
	defer cleanup()

	repo := NewMySQLMessageReactionRepository(db)
	ctx := context.Background()

	if err := repo.AddMessageReaction(ctx, 12, 1, "👍"); err != nil {
		t.Fatalf("AddMessageReaction failed: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM messages WHERE id = 12`); err != nil {
		t.Fatalf("failed to delete the message: %v", err)
	}

	got, err := repo.ListMessageReactionsByMessageIDs(ctx, []int64{12}, 1)
	if err != nil {
		t.Fatalf("ListMessageReactionsByMessageIDs failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want the reactions to go away with the message (ON DELETE CASCADE)", got)
	}
}
