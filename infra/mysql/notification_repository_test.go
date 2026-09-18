package mysql

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

func broadcastTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	return throwawaySchemaDB(t, "space_notification_test", []string{
		`CREATE TABLE users (
			id BIGINT NOT NULL AUTO_INCREMENT,
			status VARCHAR(50) NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE notifications (
			id BIGINT NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			type VARCHAR(50) NOT NULL,
			actor_id BIGINT NULL,
			target_type VARCHAR(50) NULL,
			target_id BIGINT NULL,
			message TEXT NOT NULL,
			is_read BOOLEAN NOT NULL DEFAULT FALSE,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (id)
		)`,
	})
}

// お知らせの展開は INSERT ... SELECT 1本で「アクティブな利用者ぶんだけ」行を作る。
// 宛先の決まり方（status = 'active'）は、これを置き換えた ListAllUserIDs と同じ。
// アプリへ利用者IDを一切運ばないので、Go 側からは行数でしか確かめられない。
func TestSaveForAllActiveUsers_CreatesOneRowPerActiveUser(t *testing.T) {
	db, cleanup := broadcastTestDB(t)
	defer cleanup()

	ctx := context.Background()
	for _, status := range []string{"active", "active", "frozen", "active", "withdrawn"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO users (status) VALUES (?)`, status); err != nil {
			t.Fatalf("failed to seed a user: %v", err)
		}
	}

	repo := &MySQLNotificationRepository{DB: db}
	targetType := "announcement"
	targetID := int64(42)

	created, err := repo.SaveForAllActiveUsers(ctx, repository.BroadcastNotificationParam{
		Type:       "announcement",
		TargetType: &targetType,
		TargetID:   &targetID,
		Message:    "新しいお知らせ",
		CreatedAt:  1700000000,
	})
	if err != nil {
		t.Fatalf("SaveForAllActiveUsers returned an error: %v", err)
	}
	if created != 3 {
		t.Fatalf("expected 3 rows for the 3 active users, got %d", created)
	}

	var rows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications`).Scan(&rows); err != nil {
		t.Fatalf("failed to count notifications: %v", err)
	}
	if rows != 3 {
		t.Fatalf("expected 3 notification rows, got %d", rows)
	}

	// 凍結・退会した利用者には作らない。
	var forInactive int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM notifications n
		JOIN users u ON u.id = n.user_id
		WHERE u.status <> 'active'`).Scan(&forInactive); err != nil {
		t.Fatalf("failed to count notifications for inactive users: %v", err)
	}
	if forInactive != 0 {
		t.Fatalf("expected no notifications for inactive users, got %d", forInactive)
	}

	// 配信のための引き直しは「接続中の利用者ぶんだけ」に絞られる。
	got, err := repo.ListByTargetForUsers(ctx, targetType, targetID, []int64{2, 4})
	if err != nil {
		t.Fatalf("ListByTargetForUsers returned an error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows scoped to the connected users, got %d", len(got))
	}
	for _, n := range got {
		if n.UserID != 2 && n.UserID != 4 {
			t.Fatalf("looked up a row for an unrequested user: %d", n.UserID)
		}
		if n.ID == 0 {
			t.Fatal("the delivery payload needs the notification id, but it came back zero")
		}
		if n.IsRead {
			t.Fatal("a freshly created notification must not be read")
		}
	}

	// 接続者がいなければ DB へは行かない。
	none, err := repo.ListByTargetForUsers(ctx, targetType, targetID, nil)
	if err != nil {
		t.Fatalf("ListByTargetForUsers with no users returned an error: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no rows, got %d", len(none))
	}
}
