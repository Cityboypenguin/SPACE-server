package mysql

import (
	"context"
	"testing"

	appdb "github.com/Cityboypenguin/SPACE-server/db"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 参加しているコミュニティは、最後に動きがあった順に並ぶ（既読かどうかは関係ない）。
func TestListCommunitiesByUserIDOrdersByLatestActivity(t *testing.T) {
	db, clean := throwawaySchemaDB(t, "community_order_test", nil)
	defer clean()
	if err := appdb.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO users (id, created_at) VALUES (1, 0), (2, 0)")
	exec(`INSERT INTO user_accounts (user_id, account_id, name, email, hashed_password, updated_at)
		VALUES (1, 'a', 'A', 'a@example.com', 'x', 0), (2, 'b', 'B', 'b@example.com', 'x', 0)`)
	communities := NewMySQLCommunityRepository(db)
	ids := map[string]int64{}
	rooms := map[string]int64{}
	for _, name := range []string{"old", "quiet", "busy"} {
		c, err := communities.SaveCommunityWithRoom(ctx, name, "", nil, 2)
		if err != nil {
			t.Fatal(err)
		}
		ids[name], rooms[name] = c.ID, c.RoomID
	}
	// 1 が参加した順は old → quiet → busy。
	for i, name := range []string{"old", "quiet", "busy"} {
		exec("INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (?, 1, 'member', ?, ?)",
			rooms[name], 100+i, 100+i)
	}
	// old に新しいメッセージが来た。busy にはそれより前のメッセージだけ。quiet は無し。
	exec("INSERT INTO messages (room_id, user_id, content, created_at, updated_at) VALUES (?, 2, 'hi', 500, 500)", rooms["old"])
	exec("INSERT INTO messages (room_id, user_id, content, created_at, updated_at) VALUES (?, 2, 'hi', 300, 300)", rooms["busy"])

	got, total, err := communities.ListCommunitiesByUserID(ctx, 1, repository.PageQuery{Limit: 10, WithTotal: true})
	if err != nil || total != 3 {
		t.Fatalf("list: total=%d err=%v", total, err)
	}
	want := []string{"old", "busy", "quiet"}
	for i, name := range want {
		if got[i].ID != ids[name] {
			t.Fatalf("order[%d] = %s, want %v", i, got[i].Name, want)
		}
	}
}

// DM も、最後にメッセージが来た順に並ぶ。
func TestListDMRoomsByUserIDOrdersByLatestMessage(t *testing.T) {
	db, clean := throwawaySchemaDB(t, "dm_order_test", nil)
	defer clean()
	if err := appdb.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO users (id, created_at) VALUES (1, 0), (2, 0), (3, 0), (4, 0)")
	exec(`INSERT INTO user_accounts (user_id, account_id, name, email, hashed_password, updated_at) VALUES
		(1, 'a', 'A', 'a@example.com', 'x', 0), (2, 'b', 'B', 'b@example.com', 'x', 0),
		(3, 'c', 'C', 'c@example.com', 'x', 0), (4, 'd', 'D', 'd@example.com', 'x', 0)`)
	// 作った順は 10 → 11 → 12。11 に一番新しいメッセージ、10 にはそれより前のメッセージ。
	for i, room := range []int64{10, 11, 12} {
		exec("INSERT INTO rooms (id, name, type, created_at, updated_at) VALUES (?, '', 'dm', ?, ?)", room, 100+i, 100+i)
		exec("INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (?, 1, 'member', 0, 0), (?, ?, 'member', 0, 0)",
			room, room, int64(2+i))
	}
	exec("INSERT INTO messages (room_id, user_id, content, created_at, updated_at) VALUES (11, 3, 'hi', 500, 500), (10, 2, 'hi', 300, 300)")

	got, _, err := NewMySQLRoomUserRepository(db).ListDMRoomsByUserID(context.Background(), 1, repository.PageQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{11, 10, 12}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("order[%d] = %d, want %v", i, got[i].ID, want)
		}
	}
}
