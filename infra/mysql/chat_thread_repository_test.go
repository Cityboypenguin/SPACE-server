package mysql

import (
	"context"
	"encoding/base64"
	"errors"
	migrationdb "github.com/Cityboypenguin/SPACE-server/db"
	"github.com/Cityboypenguin/SPACE-server/internal/messagecrypto"
	"github.com/Cityboypenguin/SPACE-server/model"
	"os"
	"strings"
	"testing"
	"time"
)

func TestChatThreadPersistenceAndMigration(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_threads", []string{
		`CREATE TABLE users(id BIGINT NOT NULL PRIMARY KEY)`,
		`CREATE TABLE rooms(id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,name VARCHAR(255) NOT NULL,type VARCHAR(50) NOT NULL,created_at BIGINT NOT NULL,updated_at BIGINT NOT NULL)`,
		`CREATE TABLE messages(id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,room_id BIGINT NOT NULL,user_id BIGINT NOT NULL,author_role VARCHAR(20) NOT NULL,content TEXT NOT NULL,reply_to_message_id BIGINT NULL,created_at BIGINT NOT NULL,updated_at BIGINT NOT NULL,deleted_at BIGINT NULL,deleted_by BIGINT NULL,FOREIGN KEY(room_id) REFERENCES rooms(id) ON DELETE CASCADE,FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE)`,
		`CREATE TABLE course_room_reads(room_id BIGINT NOT NULL,user_id BIGINT NOT NULL,last_read_at BIGINT NULL,last_read_message_id BIGINT NULL,PRIMARY KEY(room_id,user_id))`,
	})
	defer cleanup()
	apply := func(name string) {
		t.Helper()
		data, err := os.ReadFile("../../db/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range strings.Split(string(data), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	apply("110_add_thread_parent_to_rooms.up.sql")
	apply("111_create_chat_threads.up.sql")
	apply("112_add_thread_link_to_messages.up.sql")
	apply("115_add_stamp_id_to_messages.up.sql")
	ctx := context.Background()
	rooms := NewMySQLRoomRepository(db)
	threads := &MySQLChatThreadRepository{DB: db}
	cipher, err := messagecrypto.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	messages := &MySQLMessageRepository{DB: db, cipher: cipher}
	tx := NewMySQLTxManager(db)
	if _, err := db.Exec("INSERT INTO users VALUES (10),(11)"); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	parent := &model.Room{Name: "親", Type: model.RoomTypeCommunity, CreatedAt: now, UpdatedAt: now}
	if err := rooms.SaveRoom(ctx, parent); err != nil {
		t.Fatal(err)
	}
	child := &model.Room{Name: "相談", Type: parent.Type, ParentRoomID: &parent.ID, CreatedAt: now, UpdatedAt: now}
	var announcement *model.Message
	if err := tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := rooms.SaveRoom(ctx, child); err != nil {
			return err
		}
		if err := threads.Create(ctx, child.ID, 10); err != nil {
			return err
		}
		announcement = &model.Message{RoomID: parent.ID, UserID: 10, AuthorRole: model.AuthorRoleStudent, ThreadRoomID: &child.ID, Content: "スレッドを作成しました", CreatedAt: now, UpdatedAt: now}
		return messages.SaveMessage(ctx, announcement)
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := rooms.GetRoomsByIDs(ctx, []int64{parent.ID, child.ID})
	if err != nil {
		t.Fatal(err)
	}
	if loaded[child.ID].PolicyRoomID() != parent.ID || *loaded[child.ID].ThreadCreatorID != 10 {
		t.Fatal("thread metadata did not round trip")
	}
	saved, err := messages.GetMessageByID(ctx, announcement.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ThreadRoomID == nil || *saved.ThreadRoomID != child.ID || saved.Content != announcement.Content {
		t.Fatal("encrypted system announcement did not round trip")
	}
	// Two messages created in the same second: the read ID, rather than time, is decisive.
	first := &model.Message{RoomID: child.ID, UserID: 11, AuthorRole: model.AuthorRoleStudent, Content: "first", CreatedAt: now, UpdatedAt: now}
	if err := messages.SaveMessage(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := *first
	second.ID = 0
	second.Content = "second"
	if err := messages.SaveMessage(ctx, &second); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO course_room_reads VALUES(?,?,?,?)", child.ID, 10, now.Unix(), first.ID); err != nil {
		t.Fatal(err)
	}
	list, err := threads.List(ctx, parent.ID, 10, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].MessageCount != 2 || list[0].UnreadCount != 1 {
		t.Fatalf("incorrect thread counts: %+v", list)
	}

	// Put the unread thread beyond the first page: aggregate badges must still find it.
	for i := 0; i < 20; i++ {
		newer := &model.Room{Name: "newer", Type: parent.Type, ParentRoomID: &parent.ID, CreatedAt: time.Unix(2000+int64(i), 0), UpdatedAt: now}
		if err := rooms.SaveRoom(ctx, newer); err != nil {
			t.Fatal(err)
		}
		if err := threads.Create(ctx, newer.ID, 10); err != nil {
			t.Fatal(err)
		}
	}
	page, err := threads.List(ctx, parent.ID, 10, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page {
		if item.UnreadCount != 0 {
			t.Fatal("first page should contain only read threads")
		}
	}
	unread, err := threads.HasUnread(ctx, parent.ID, 10)
	if err != nil || !unread {
		t.Fatalf("unread on later page was missed: %t %v", unread, err)
	}
	unread, err = threads.HasUnread(ctx, parent.ID, 11)
	if err != nil || unread {
		t.Fatalf("own messages must not be unread: %t %v", unread, err)
	}
	unread, err = threads.HasUnread(ctx, parent.ID+10000, 10)
	if err != nil || unread {
		t.Fatalf("unread leaked to another parent: %t %v", unread, err)
	}
	if _, err := db.Exec("UPDATE course_room_reads SET last_read_message_id=? WHERE room_id=? AND user_id=10", second.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	unread, err = threads.HasUnread(ctx, parent.ID, 10)
	if err != nil || unread {
		t.Fatalf("badge remained after marking read: %t %v", unread, err)
	}
	deleted := second
	deleted.ID = 0
	if err := messages.SaveMessage(ctx, &deleted); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE messages SET deleted_at=2000 WHERE id=?", deleted.ID); err != nil {
		t.Fatal(err)
	}
	unread, err = threads.HasUnread(ctx, parent.ID, 10)
	if err != nil || unread {
		t.Fatalf("deleted messages must not be unread: %t %v", unread, err)
	}
	if err := tx.RunInTx(ctx, func(ctx context.Context) error { return threads.Update(ctx, child.ID, "新タイトル", true) }); err != nil {
		t.Fatal(err)
	}
	detail, err := threads.Get(ctx, child.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Room.Name != "新タイトル" || detail.Room.ThreadClosedAt == nil {
		t.Fatal("title and close state did not persist")
	}
	// A failed creation must not leave a room or its metadata behind.
	failed := &model.Room{Name: "失敗", Type: parent.Type, ParentRoomID: &parent.ID, CreatedAt: now, UpdatedAt: now}
	expected := errors.New("injected failure")
	if err := tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := rooms.SaveRoom(ctx, failed); err != nil {
			return err
		}
		if err := threads.Create(ctx, failed.ID, 10); err != nil {
			return err
		}
		return expected
	}); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	if r, err := rooms.GetRoomByID(ctx, failed.ID); err != nil || r != nil {
		t.Fatal("transaction did not roll back")
	}
	// Deleting the parent must remove child rooms, metadata, posts and read positions.
	if _, err := rooms.DeleteRoom(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if r, err := rooms.GetRoomByID(ctx, child.ID); err != nil || r != nil {
		t.Fatal("parent deletion left an orphan thread")
	}
	apply("112_add_thread_link_to_messages.down.sql")
	apply("111_create_chat_threads.down.sql")
	apply("110_add_thread_parent_to_rooms.down.sql")
	apply("110_add_thread_parent_to_rooms.up.sql")
	apply("111_create_chat_threads.up.sql")
	apply("112_add_thread_link_to_messages.up.sql")
}

func TestChatThreadFullMigration(t *testing.T) {
	database, cleanup := throwawaySchemaDB(t, "space_threads_full", nil)
	defer cleanup()
	if err := migrationdb.RunMigrations(database); err != nil {
		t.Fatal(err)
	}
}
