//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/db"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// これらのテストは実MySQLが必要。`make test-integration-up` でDBを起動してから
// `make test-integration` で実行する(通常の `go test ./...` では -tags=integration
// が無いためコンパイル対象外になり、スキップされる)。

func setupIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := New()
	if err != nil {
		t.Skipf("skipping integration test: cannot connect to test database: %v", err)
	}
	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return sqlDB
}

// seedRoomAndUser inserts a minimal room and user row to satisfy messages'
// foreign keys, using a name unique per test run so repeated runs against a
// long-lived database don't collide on users' unique account_id/email.
func seedRoomAndUser(t *testing.T, sqlDB *sql.DB) (roomID, userID int64) {
	t.Helper()
	now := time.Now().Unix()
	unique := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())

	res, err := sqlDB.Exec(`INSERT INTO rooms (name, type, created_at, updated_at) VALUES (?, 'course', ?, ?)`, unique, now, now)
	if err != nil {
		t.Fatalf("seed room: %v", err)
	}
	roomID, _ = res.LastInsertId()

	res, err = sqlDB.Exec(`INSERT INTO users (account_id, name, email, hashed_password, role, status, created_at, updated_at)
		VALUES (?, ?, ?, 'x', 'student', 'active', ?, ?)`, unique, unique, unique+"@example.test", now, now)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	userID, _ = res.LastInsertId()
	return roomID, userID
}

func newIntegrationMessageRepo(t *testing.T, sqlDB *sql.DB) *MySQLMessageRepository {
	t.Helper()
	repo, err := NewMySQLMessageRepository(sqlDB)
	if err != nil {
		t.Fatalf("NewMySQLMessageRepository: %v", err)
	}
	return repo
}

// 項番45: 絵文字・サロゲートペアがDB保存/取得を経ても文字化けしない。
func TestIntegration_SaveAndGetMessage_PreservesEmoji(t *testing.T) {
	sqlDB := setupIntegrationDB(t)
	repo := newIntegrationMessageRepo(t, sqlDB)
	roomID, userID := seedRoomAndUser(t, sqlDB)
	ctx := context.Background()

	const content = "😀👍🏽 テスト絵文字メッセージ"
	now := time.Now()
	msg := &model.Message{RoomID: roomID, UserID: userID, Content: content, CreatedAt: now, UpdatedAt: now}
	if err := repo.SaveMessage(ctx, msg); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	got, err := repo.GetMessageByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetMessageByID: %v", err)
	}
	if got == nil || got.Content != content {
		t.Fatalf("content = %q, want %q", got.Content, content)
	}

	// カラムの生値は平文ではなく暗号化されていること(暗号化されていることの直接確認)。
	var rawContent string
	if err := sqlDB.QueryRowContext(ctx, "SELECT content FROM messages WHERE id = ?", msg.ID).Scan(&rawContent); err != nil {
		t.Fatalf("read raw content: %v", err)
	}
	if rawContent == content {
		t.Fatal("raw column value must not equal the plaintext content (encryption not applied)")
	}
}

// 項番54,55: 論理削除されたメッセージは一般読み取り経路(GetMessageByID /
// ListMessagesByRoomID / GetLastMessagesByRoomIDs)から完全に除外される。
func TestIntegration_SoftDeletedMessage_HiddenFromAllReadPaths(t *testing.T) {
	sqlDB := setupIntegrationDB(t)
	repo := newIntegrationMessageRepo(t, sqlDB)
	roomID, userID := seedRoomAndUser(t, sqlDB)
	ctx := context.Background()

	now := time.Now()
	msg := &model.Message{RoomID: roomID, UserID: userID, Content: "hello", CreatedAt: now, UpdatedAt: now}
	if err := repo.SaveMessage(ctx, msg); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	deleted, err := repo.SoftDeleteMessage(ctx, msg.ID, userID)
	if err != nil || !deleted {
		t.Fatalf("SoftDeleteMessage: deleted=%v err=%v", deleted, err)
	}

	if got, err := repo.GetMessageByID(ctx, msg.ID); err != nil || got != nil {
		t.Fatalf("GetMessageByID should hide soft-deleted message, got %v err=%v", got, err)
	}

	list, _, _, err := repo.ListMessagesByRoomID(ctx, roomID, 50, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListMessagesByRoomID: %v", err)
	}
	for _, m := range list {
		if m.ID == msg.ID {
			t.Fatal("soft-deleted message must not appear in room listing")
		}
	}

	last, err := repo.GetLastMessagesByRoomIDs(ctx, []int64{roomID})
	if err != nil {
		t.Fatalf("GetLastMessagesByRoomIDs: %v", err)
	}
	if lm, ok := last[roomID]; ok && lm.ID == msg.ID {
		t.Fatal("soft-deleted message must not be reported as the room's last message")
	}
}

// 項番40: 同一メッセージへの2回目の SoftDeleteMessage は false を返し、
// 1回目の deletedBy を上書きしない。
func TestIntegration_SoftDeleteMessage_SecondCallIsNoop(t *testing.T) {
	sqlDB := setupIntegrationDB(t)
	repo := newIntegrationMessageRepo(t, sqlDB)
	roomID, userID := seedRoomAndUser(t, sqlDB)
	_, otherUserID := seedRoomAndUser(t, sqlDB)
	ctx := context.Background()

	now := time.Now()
	msg := &model.Message{RoomID: roomID, UserID: userID, Content: "hello", CreatedAt: now, UpdatedAt: now}
	if err := repo.SaveMessage(ctx, msg); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	first, err := repo.SoftDeleteMessage(ctx, msg.ID, userID)
	if err != nil || !first {
		t.Fatalf("first delete: deleted=%v err=%v", first, err)
	}
	second, err := repo.SoftDeleteMessage(ctx, msg.ID, otherUserID)
	if err != nil {
		t.Fatalf("second delete: unexpected error: %v", err)
	}
	if second {
		t.Fatal("second SoftDeleteMessage call must report false (already deleted)")
	}

	var deletedBy sql.NullInt64
	if err := sqlDB.QueryRowContext(ctx, "SELECT deleted_by FROM messages WHERE id = ?", msg.ID).Scan(&deletedBy); err != nil {
		t.Fatalf("read deleted_by: %v", err)
	}
	if !deletedBy.Valid || deletedBy.Int64 != userID {
		t.Fatalf("deleted_by = %v, want unchanged first deleter %d", deletedBy, userID)
	}
}
