package db

import "testing"

func TestChatThreadMigrationFromDevelop(t *testing.T) {
	database := throwawayDB(t)
	migrator := newMigrator(t, database)
	if err := migrator.Migrate(73); err != nil {
		t.Fatal(err)
	}
	seedBeforeSplit(t, database)
	if err := migrator.Migrate(109); err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(112); err != nil {
		t.Fatal(err)
	}
	version, dirty, err := migrator.Version()
	if err != nil || dirty || version != 112 {
		t.Fatalf("version=%d dirty=%t err=%v", version, dirty, err)
	}
	if count(t, database, "SELECT COUNT(*) FROM messages WHERE id=100") != 1 || count(t, database, "SELECT COUNT(*) FROM user_accounts") != 2 {
		t.Fatal("thread migration lost Develop content")
	}
	mustExec(t, database, `INSERT INTO rooms (id,name,type,parent_room_id,created_at,updated_at) VALUES(11,'コミュニティ','community',NULL,1,1),(12,'相談','community',11,1,1)`)
	mustExec(t, database, `INSERT INTO chat_threads(room_id,created_by) VALUES(12,1)`)
	mustExec(t, database, `INSERT INTO messages(id,room_id,user_id,content,created_at,updated_at,thread_room_id) VALUES(101,11,1,'スレッドを作成しました',1,1,12),(102,12,1,'会話',1,1,NULL)`)
	mustExec(t, database, `INSERT INTO course_room_reads(room_id,user_id,last_read_at,created_at,updated_at,last_read_message_id) VALUES(12,1,1,1,1,102)`)
	// Develop removes personal accounts but retains authored conversations and identities.
	mustExec(t, database, `DELETE FROM user_accounts WHERE user_id=1`)
	mustExec(t, database, `UPDATE users SET status='deleted',deleted_at=2 WHERE id=1`)
	if count(t, database, "SELECT COUNT(*) FROM chat_threads WHERE room_id=12 AND created_by=1") != 1 || count(t, database, "SELECT COUNT(*) FROM messages WHERE id IN (101,102)") != 2 {
		t.Fatal("account erasure lost thread history")
	}
	if count(t, database, "SELECT COUNT(*) FROM course_room_reads WHERE room_id=12") != 0 {
		t.Fatal("account erasure retained personal thread read positions")
	}
}
