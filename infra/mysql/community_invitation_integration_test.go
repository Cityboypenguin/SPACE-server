package mysql

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	appdb "github.com/Cityboypenguin/SPACE-server/db"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/usecase/chat"
	"github.com/Cityboypenguin/SPACE-server/usecase/community"
	"github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

type invitationDelivery struct{}

func (invitationDelivery) PublishToUser(int64, string, map[string]any) {}
func (invitationDelivery) ConnectedUserIDs() []int64                   { return nil }

func TestCommunityInvitationParticipation(t *testing.T) {
	db, clean := throwawaySchemaDB(t, "community_invite_test", nil)
	defer clean()
	if err := appdb.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	as := func(id int64, role string) context.Context {
		return auth.WithClaims(ctx, &auth.Claims{ID: id, Role: role})
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for id := int64(1); id <= 6; id++ {
		account := fmt.Sprintf("test%d", id)
		exec("INSERT INTO users (id, created_at) VALUES (?, 0)", id)
		exec("INSERT INTO user_accounts (user_id, account_id, name, email, hashed_password, updated_at) VALUES (?, ?, ?, ?, 'test', 0)",
			id, account, account, account+"@example.com")
	}
	t.Setenv("MESSAGE_ENCRYPTION_KEY", "12345678901234567890123456789012")
	messages, err := NewMySQLMessageRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txs := NewMySQLTxManager(db)
	communities := NewMySQLCommunityRepository(db)
	stamps := &ChatStampRepository{DB: db}
	service := &community.InvitationService{
		Store: &CommunityInvitationRepository{DB: db}, Rooms: NewMySQLRoomRepository(db),
		Members: NewMySQLRoomUserRepository(db), Communities: communities, Users: NewMySQLUserRepository(db),
		Blocks: NewMySQLBlockRepository(db), Tx: txs, Stamps: stamps,
		Notifications: notification.NewNotificationPublisher(NewMySQLNotificationRepository(db), invitationDelivery{}),
		Messages:      chat.NewMessageWriters(nil, messages, messages, messages, nil, txs, nil),
		Events:        chat.NoopEventPublisher{},
	}
	group, err := communities.SaveCommunityWithRoom(ctx, "コミュニティ", "説明", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	admin := as(1, "admin")
	owner := as(1, "student")
	member := as(2, "student")
	other := as(3, "student")

	settings, err := service.AdminSettings(admin)
	if err != nil || len(settings.GreetingIDs) != 0 || len(settings.WelcomeIDs) != 0 {
		t.Fatalf("initial stamp settings: %+v, %v", settings, err)
	}
	for _, id := range []string{"stamp:wave", "stamp:hello"} {
		if err := stamps.Create(ctx, &model.ChatStamp{
			ID: id, Label: id, Status: "active", DisplaySize: "standard", AllowMessage: true, CreatedBy: 1, UpdatedBy: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	settings = model.CommunityJoinStampSettings{
		WelcomeIDs: []string{"stamp:wave", "stamp:hello"}, GreetingIDs: []string{"stamp:hello", "stamp:wave"},
	}
	if err := service.SaveSettings(member, settings); err == nil {
		t.Fatal("non-admin updated the settings")
	}
	if err := service.SaveSettings(admin, settings); err != nil {
		t.Fatal(err)
	}

	recipient := int64(2)
	if _, _, err := service.Send(other, group.ID, &recipient); err == nil {
		t.Fatal("non-member sent an invitation")
	}
	exec("INSERT INTO blocks (user_id, blocked_user_id, created_at) VALUES (2, 1, 0)")
	if _, _, err := service.Send(owner, group.ID, &recipient); err == nil {
		t.Fatal("a blocked invitation was sent")
	}
	exec("DELETE FROM blocks")
	invitation, _, err := service.Send(owner, group.ID, &recipient)
	if err != nil {
		t.Fatal(err)
	}
	retry, _, err := service.Send(owner, group.ID, &recipient)
	if err != nil || retry.ID != invitation.ID {
		t.Fatalf("individual invite retry: %+v, %v", retry, err)
	}
	var notificationCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = 2 AND type = 'community_invitation' AND target_id = ?", invitation.ID).Scan(&notificationCount); err != nil || notificationCount != 1 {
		t.Fatalf("notifications count=%d, err=%v", notificationCount, err)
	}
	if _, err := service.Preview(other, invitation.ID, ""); err == nil {
		t.Fatal("another user viewed an individual invitation")
	}
	if items, count, err := service.Received(member, 20, 0); err != nil || count != 1 || len(items) != 1 {
		t.Fatalf("received: %v, count=%d, err=%v", items, count, err)
	}
	exec("INSERT INTO blocks (user_id, blocked_user_id, created_at) VALUES (1, 2, 0)")
	if _, err := service.Respond(member, invitation.ID, "", true); err == nil {
		t.Fatal("accepted after a block")
	}
	if _, count, err := service.Received(member, 0, 0); err != nil || count != 0 {
		t.Fatalf("blocked invitation count=%d, err=%v", count, err)
	}
	exec("DELETE FROM blocks")

	// The two entry points may race. Exactly one membership and welcome message should survive.
	var wg sync.WaitGroup
	joinErr := make(chan error, 2)
	wg.Go(func() { _, err := service.Execute(member, group.RoomID); joinErr <- err })
	wg.Go(func() { _, err := service.Respond(member, invitation.ID, "", true); joinErr <- err })
	wg.Wait()
	close(joinErr)
	for err := range joinErr {
		if err != nil {
			t.Fatal(err)
		}
	}
	checkMessages := func(authorRole string, want int) {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE room_id = ? AND author_role = ? AND user_id = 2", group.RoomID, authorRole).Scan(&n); err != nil || n != want {
			t.Fatalf("role=%q count=%d want=%d err=%v", authorRole, n, want, err)
		}
	}
	checkMessages(model.AuthorRoleCommunityWelcome, 1)
	if _, count, err := service.Received(member, 0, 0); err != nil || count != 0 {
		t.Fatalf("accepted invitation count=%d, err=%v", count, err)
	}

	welcome, options, err := service.Greeting(member, group.RoomID)
	if err != nil || welcome == nil || len(options) != 2 || options[0].ID != "stamp:hello" {
		t.Fatalf("greeting ID=%v options=%v err=%v", welcome, options, err)
	}
	welcomeMessage, err := messages.GetMessageByID(ctx, *welcome)
	if err != nil || welcomeMessage == nil || welcomeMessage.StampID == nil ||
		(*welcomeMessage.StampID != "stamp:wave" && *welcomeMessage.StampID != "stamp:hello") {
		t.Fatalf("welcome message=%+v, err=%v", welcomeMessage, err)
	}
	if _, err := service.SendGreeting(other, group.RoomID, *welcome, options[0].ID); err == nil {
		t.Fatal("another user sent the greeting")
	}
	if _, err := service.SendGreeting(member, group.RoomID, *welcome, "stamp:other"); err == nil {
		t.Fatal("a greeting stamp outside the configured candidates was accepted")
	}
	if _, err := service.SendGreeting(member, group.RoomID, *welcome, options[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SendGreeting(member, group.RoomID, *welcome, options[1].ID); err == nil {
		t.Fatal("the greeting was sent twice")
	}
	checkMessages(model.AuthorRoleStudent, 1)

	link, token, err := service.Send(member, group.ID, nil)
	if err != nil || len(token) != 43 {
		t.Fatalf("create link: id=%v err=%v", link, err)
	}
	for _, id := range []int64{3, 4} {
		roomID, err := service.Respond(as(id, "student"), 0, token, true)
		if err != nil || roomID != group.RoomID {
			t.Fatalf("redeem link as %d: room=%d err=%v", id, roomID, err)
		}
	}
	if err := service.Revoke(owner, link.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Respond(as(5, "student"), 0, token, true); err == nil {
		t.Fatal("revoked link was redeemed")
	}
	_, expiredToken, err := service.Send(member, group.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE community_invitations SET expires_at = ? WHERE room_id = ? AND state = 'pending'", time.Now().Add(-time.Second).Unix(), group.RoomID)
	if _, err := service.Respond(as(6, "student"), 0, expiredToken, true); err == nil {
		t.Fatal("expired link was redeemed")
	}
}
