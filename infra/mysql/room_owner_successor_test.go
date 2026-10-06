package mysql

import (
	"context"
	"testing"
)

// オーナーを引き継ぐのは、低浮上の人を避けて、そのコミュニティで最近発言した人。
// 発言が無ければアプリへの最終アクセス、それも無ければ参加の早さで決める。
// 退会手続き中・凍結中の人は、利用中の人が居ない時だけ選ぶ。
func TestFindOwnerSuccessorPrefersRecentlyActiveMember(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_owner_successor_test", []string{
		`CREATE TABLE users (id BIGINT NOT NULL PRIMARY KEY, status VARCHAR(50) NOT NULL DEFAULT 'active') ENGINE=InnoDB`,
		`CREATE TABLE user_accounts (user_id BIGINT NOT NULL PRIMARY KEY, last_active_at BIGINT NULL) ENGINE=InnoDB`,
		`CREATE TABLE room_users (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			role VARCHAR(50) NOT NULL DEFAULT 'member',
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY unique_room_user (room_id, user_id)
		) ENGINE=InnoDB`,
		`CREATE TABLE messages (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			room_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			deleted_at BIGINT NULL
		) ENGINE=InnoDB`,
	})
	defer cleanup()
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO users (id, status) VALUES (1, 'active'), (2, 'deactivated'), (3, 'active'), (4, 'active'), (5, 'frozen'), (6, 'active'), (7, 'active')`,
		`INSERT INTO user_accounts (user_id, last_active_at) VALUES (1, 900), (2, 900), (3, 500), (4, 100), (5, 100), (6, 800), (7, NULL)`,
		// ルーム10: 参加が最も早い 4 は低浮上。最後に発言したのが最も新しいのは 3。
		// 2 はさらに新しく発言しているが退会手続き中。6 の発言は消されている。
		`INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES
			(10, 1, 'owner', 100, 100), (10, 2, 'member', 110, 110), (10, 3, 'member', 130, 130),
			(10, 4, 'member', 105, 105), (10, 6, 'member', 140, 140)`,
		`INSERT INTO messages (room_id, user_id, created_at, deleted_at) VALUES
			(10, 4, 200, NULL), (10, 3, 300, NULL), (10, 3, 250, NULL), (10, 2, 400, NULL), (10, 6, 500, 510),
			(50, 4, 999, NULL)`,
		// ルーム20: このルームで誰も発言していないので、最終アクセスが新しい 6（ルーム外の発言は数えない）。
		`INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES
			(20, 1, 'owner', 100, 100), (20, 4, 'member', 110, 110), (20, 6, 'member', 150, 150), (20, 7, 'member', 105, 105)`,
		// ルーム30: 利用中の人が他に居ないので、残りから同じ順で選ぶ（最終アクセスが新しい 2）。
		`INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES
			(30, 1, 'owner', 100, 100), (30, 2, 'member', 150, 150), (30, 5, 'member', 140, 140)`,
		// ルーム40: 他に誰も居ない。
		`INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (40, 1, 'owner', 100, 100)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	repo := NewMySQLRoomUserRepository(db)
	for _, tt := range []struct {
		room int64
		want int64
	}{{10, 3}, {20, 6}, {30, 2}, {40, 0}} {
		got, err := repo.FindOwnerSuccessor(ctx, tt.room, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Fatalf("room %d: successor = %d, want %d", tt.room, got, tt.want)
		}
	}
}
