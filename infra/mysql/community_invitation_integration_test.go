package mysql

import (
	"context"
	"fmt"
	"strings"
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
	for _, id := range []string{"stamp:wave", "stamp:hello", "stamp:auto"} {
		if err := stamps.Create(ctx, &model.ChatStamp{
			ID: id, Label: id, Status: "active", DisplaySize: "standard", AllowMessage: true, CreatedBy: 1, UpdatedBy: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	settings = model.CommunityJoinStampSettings{
		WelcomeIDs: []string{"stamp:wave", "stamp:hello"}, GreetingIDs: []string{"stamp:auto"},
	}
	if err := service.SaveSettings(member, settings); err == nil {
		t.Fatal("non-admin updated the settings")
	}
	if err := service.SaveSettings(admin, settings); err != nil {
		t.Fatal(err)
	}

	recipient := int64(2)
	if _, _, err := service.Send(other, group.ID, &recipient, model.CommunityInviteOptions{}); err == nil {
		t.Fatal("non-member sent an invitation")
	}
	exec("INSERT INTO blocks (user_id, blocked_user_id, created_at) VALUES (2, 1, 0)")
	if _, _, err := service.Send(owner, group.ID, &recipient, model.CommunityInviteOptions{}); err == nil {
		t.Fatal("a blocked invitation was sent")
	}
	exec("DELETE FROM blocks")
	if states, err := service.Candidates(owner, group.ID, []*model.User{{ID: 1}, {ID: 2}}); err != nil ||
		states[0].State != model.CommunityCandidateMember || states[1].State != model.CommunityCandidateAvailable {
		t.Fatalf("candidates before invite: %+v, %v", states, err)
	}
	long := strings.Repeat("あ", 201)
	if _, _, err := service.Send(owner, group.ID, &recipient, model.CommunityInviteOptions{Message: &long}); err == nil {
		t.Fatal("an over-long message was accepted")
	}
	note := "  一緒に話しましょう  "
	invitation, _, err := service.Send(owner, group.ID, &recipient, model.CommunityInviteOptions{Message: &note})
	if err != nil {
		t.Fatal(err)
	}
	if stored, err := service.Preview(member, invitation.ID, ""); err != nil || stored.Message == nil || *stored.Message != "一緒に話しましょう" {
		t.Fatalf("invitation message: %+v, %v", stored, err)
	}
	if states, err := service.Candidates(owner, group.ID, []*model.User{{ID: 2}}); err != nil || states[0].State != model.CommunityCandidateInvited {
		t.Fatalf("candidates after invite: %+v, %v", states, err)
	}
	if _, err := service.Candidates(other, group.ID, []*model.User{{ID: 2}}); err == nil {
		t.Fatal("a non-member listed invite candidates")
	}
	retry, _, err := service.Send(owner, group.ID, &recipient, model.CommunityInviteOptions{})
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
	if active, err := service.Sent(owner, group.RoomID, false, 20, 0); err != nil || len(active) != 0 {
		t.Fatalf("active after accept: %v, %v", active, err)
	}
	if history, err := service.Sent(owner, group.RoomID, true, 20, 0); err != nil || len(history) != 1 ||
		history[0].Status(time.Now()) != model.CommunityInvitationAccepted || history[0].RespondedAt == nil {
		t.Fatalf("history after accept: %+v, %v", history, err)
	}

	target, options, err := service.Greeting(owner, group.RoomID)
	if err != nil || target == nil || target.UserName != "test2" || len(options) != 2 || options[0].ID != "stamp:wave" {
		t.Fatalf("welcome target=%+v options=%v err=%v", target, options, err)
	}
	notice, err := messages.GetMessageByID(ctx, target.MessageID)
	if err != nil || notice == nil || notice.StampID != nil || notice.AuthorRole != model.AuthorRoleCommunityWelcome {
		t.Fatalf("join notice=%+v, err=%v", notice, err)
	}
	checkMessages(model.AuthorRoleStudent, 1)
	var automaticStamp string
	if err := db.QueryRow("SELECT stamp_id FROM messages WHERE room_id = ? AND user_id = 2 AND author_role = ? AND id > ?",
		group.RoomID, model.AuthorRoleStudent, target.MessageID).Scan(&automaticStamp); err != nil || automaticStamp != "stamp:auto" {
		t.Fatalf("separate automatic stamp=%q err=%v", automaticStamp, err)
	}
	if own, _, err := service.Greeting(member, group.RoomID); err != nil || own != nil {
		t.Fatalf("new member sees their own welcome suggestions: %+v, %v", own, err)
	}
	if _, err := service.SendGreeting(other, group.RoomID, target.MessageID, options[0].ID); err == nil {
		t.Fatal("a non-member sent a welcome stamp")
	}
	if _, err := service.SendGreeting(member, group.RoomID, target.MessageID, options[0].ID); err == nil {
		t.Fatal("a new member welcomed themself")
	}
	if _, err := service.SendGreeting(owner, group.RoomID, target.MessageID, "stamp:auto"); err == nil {
		t.Fatal("a stamp outside the configured welcome candidates was accepted")
	}
	reply, err := service.SendGreeting(owner, group.RoomID, target.MessageID, options[1].ID)
	if err != nil || reply == nil || reply.UserID != 1 || reply.StampID == nil || *reply.StampID != options[1].ID {
		t.Fatalf("welcome reply=%+v err=%v", reply, err)
	}
	if _, err := service.SendGreeting(owner, group.RoomID, target.MessageID, options[1].ID); err == nil {
		t.Fatal("the welcome stamp was sent twice")
	}

	link, token, err := service.Send(member, group.ID, nil, model.CommunityInviteOptions{})
	if err != nil || len(token) != 43 {
		t.Fatalf("create link: id=%v err=%v", link, err)
	}
	for _, id := range []int64{3, 4} {
		roomID, err := service.Respond(as(id, "student"), 0, token, true)
		if err != nil || roomID != group.RoomID {
			t.Fatalf("redeem link as %d: room=%d err=%v", id, roomID, err)
		}
	}
	if stored, err := service.Preview(as(5, "student"), 0, token); err != nil || stored.UseCount != 2 {
		t.Fatalf("link use count: %+v, %v", stored, err)
	}
	var accepted int
	if err := db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = 2 AND type = 'community_invitation_accepted'").Scan(&accepted); err != nil || accepted != 2 {
		t.Fatalf("accepted notifications to the link creator=%d, err=%v", accepted, err)
	}
	if err := service.Revoke(owner, link.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Respond(as(5, "student"), 0, token, true); err == nil {
		t.Fatal("revoked link was redeemed")
	}
	if _, _, err := service.Send(member, group.ID, nil, model.CommunityInviteOptions{ExpiresIn: 5 * time.Minute}); err == nil {
		t.Fatal("an unsupported lifetime was accepted")
	}
	short, shortToken, err := service.Send(member, group.ID, nil, model.CommunityInviteOptions{ExpiresIn: time.Hour})
	if err != nil || short.ExpiresAt.Sub(short.CreatedAt) != time.Hour {
		t.Fatalf("short link: %+v, %v", short, err)
	}
	if _, err := service.Respond(as(5, "student"), 0, shortToken, true); err != nil {
		t.Fatal(err)
	}
	if history, err := service.Sent(member, group.RoomID, true, 20, 0); err != nil || len(history) != 1 ||
		history[0].Status(time.Now()) != model.CommunityInvitationRevoked || history[0].UseCount != 2 {
		t.Fatalf("link history: %+v, %v", history, err)
	}
	exec("DELETE FROM room_users WHERE room_id = ? AND user_id = 5", group.RoomID)
	results, err := service.SendMany(owner, group.ID, []int64{5, 6, 3, 5}, model.CommunityInviteOptions{Message: &note})
	if err != nil || len(results) != 3 || results[0].Err != nil || results[1].Err != nil || results[2].Err == nil {
		t.Fatalf("batch invite: %+v, %v", results, err)
	}
	_, expiredToken, err := service.Send(member, group.ID, nil, model.CommunityInviteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE community_invitations SET expires_at = ? WHERE room_id = ? AND state = 'pending'", time.Now().Add(-time.Second).Unix(), group.RoomID)
	if _, err := service.Respond(as(6, "student"), 0, expiredToken, true); err == nil {
		t.Fatal("expired link was redeemed")
	}
	exec("DELETE FROM user_accounts WHERE user_id = ?", recipient)
	var remaining int
	if err := db.QueryRow("SELECT COUNT(*) FROM community_invitations WHERE sender_id = ? OR recipient_id = ?", recipient, recipient).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("invitations after account purge=%d, err=%v", remaining, err)
	}
	checkMessages(model.AuthorRoleCommunityWelcome, 1)

}
