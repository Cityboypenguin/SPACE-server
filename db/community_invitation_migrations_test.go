package db

import "testing"

func TestCommunityInvitationMigrationsPreserveExistingChatOnRoundTrip(t *testing.T) {
	const beforeInvitations = 136
	database := throwawayDB(t)
	migrator := newMigrator(t, database)
	if err := migrator.Migrate(beforeInvitations); err != nil {
		t.Fatalf("migrate the old schema: %v", err)
	}
	mustExec(t, database, "INSERT INTO users (id, created_at) VALUES (1, 100)")
	mustExec(t, database, `INSERT INTO user_accounts
		(user_id, account_id, name, email, hashed_password, updated_at)
		VALUES (1, 'owner', 'Owner', 'owner@example.com', 'test', 100)`)
	mustExec(t, database, "INSERT INTO rooms (id, name, type, created_at, updated_at) VALUES (10, 'Group', 'community', 100, 100)")
	mustExec(t, database, "INSERT INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (10, 1, 'owner', 100, 100)")
	mustExec(t, database, "INSERT INTO messages (id, room_id, user_id, content, created_at, updated_at) VALUES (40, 10, 1, 'existing', 100, 100)")
	mustExec(t, database, "UPDATE room_users SET last_read_message_id = 40, last_read_at = 100 WHERE room_id = 10 AND user_id = 1")

	verifyChat := func() {
		t.Helper()
		if n := count(t, database, `SELECT COUNT(*) FROM room_users
			WHERE room_id = 10 AND user_id = 1 AND role = 'owner' AND last_read_message_id = 40 AND last_read_at = 100`); n != 1 {
			t.Fatal("migration changed the existing membership or read position")
		}
		if n := count(t, database, "SELECT COUNT(*) FROM messages WHERE id = 40 AND room_id = 10 AND user_id = 1 AND content = 'existing'"); n != 1 {
			t.Fatal("migration changed an existing message")
		}
	}

	if err := RunMigrations(database); err != nil {
		t.Fatalf("upgrade to invitations: %v", err)
	}
	verifyChat()
	if n := count(t, database, `SELECT COUNT(*) FROM room_users
		WHERE room_id = 10 AND user_id = 1 AND welcome_message_id IS NULL AND NOT greeting_sent`); n != 1 {
		t.Fatal("an existing membership gained a greeting")
	}
	mustExec(t, database, "INSERT INTO community_invitations (room_id, sender_id, created_at, expires_at) VALUES (10, 1, 200, 300)")
	if err := migrator.Migrate(beforeInvitations); err != nil {
		t.Fatalf("roll back invitations: %v", err)
	}
	verifyChat()
	if n := count(t, database, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME IN ('community_invitations', 'community_join_stamp_settings', 'community_welcome_replies', 'community_invitation_uses')`); n != 0 {
		t.Fatal("rollback left invitation tables behind")
	}
	if err := RunMigrations(database); err != nil {
		t.Fatalf("reapply invitations: %v", err)
	}
	verifyChat()
	if n := count(t, database, "SELECT COUNT(*) FROM community_invitations"); n != 0 {
		t.Fatal("reapplying the migration restored a rolled-back invitation")
	}
}
