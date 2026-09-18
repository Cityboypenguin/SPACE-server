package notification

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// countingNotificationRepo は「何回 DB へ行ったか」を数えるだけのリポジトリ。
// 全員宛のお知らせで往復が人数に比例しないことを確かめるために使う。
type countingNotificationRepo struct {
	repository.NotificationRepository

	broadcastCalls int
	broadcastParam repository.BroadcastNotificationParam
	lookupCalls    int
	lookupUserIDs  []int64
	rows           []*model.Notification
}

func (r *countingNotificationRepo) SaveForAllActiveUsers(_ context.Context, p repository.BroadcastNotificationParam) (int64, error) {
	r.broadcastCalls++
	r.broadcastParam = p
	return int64(len(r.rows)), nil
}

func (r *countingNotificationRepo) ListByTargetForUsers(_ context.Context, _ string, _ int64, userIDs []int64) ([]*model.Notification, error) {
	r.lookupCalls++
	r.lookupUserIDs = append([]int64(nil), userIDs...)

	wanted := make(map[int64]bool, len(userIDs))
	for _, id := range userIDs {
		wanted[id] = true
	}
	var out []*model.Notification
	for _, n := range r.rows {
		if wanted[n.UserID] {
			out = append(out, n)
		}
	}
	return out, nil
}

type recordingDelivery struct {
	connected []int64
	delivered []int64
	events    []string
}

func (d *recordingDelivery) PublishToUser(userID int64, eventType string, _ map[string]any) {
	d.delivered = append(d.delivered, userID)
	d.events = append(d.events, eventType)
}

func (d *recordingDelivery) ConnectedUserIDs() []int64 { return d.connected }

func announcementRows(userIDs ...int64) []*model.Notification {
	targetType := string(TargetAnnouncement)
	targetID := int64(7)
	rows := make([]*model.Notification, 0, len(userIDs))
	for i, userID := range userIDs {
		rows = append(rows, &model.Notification{
			ID:         int64(100 + i),
			UserID:     userID,
			Type:       string(TypeAnnouncement),
			TargetType: &targetType,
			TargetID:   &targetID,
			Message:    "新しいお知らせ",
		})
	}
	return rows
}

// 全員宛の保存は、利用者が何人いても INSERT ... SELECT 1本で済む
// （以前は「全IDを SELECT」＋「人数ぶんの VALUES を並べた INSERT」だった）。
func TestPublishToAllActiveUsers_StoresWithASingleRoundTrip(t *testing.T) {
	repo := &countingNotificationRepo{rows: announcementRows(1, 2, 3, 4, 5)}
	delivery := &recordingDelivery{}
	p := NewNotificationPublisher(repo, delivery)

	if err := p.PublishToAllActiveUsers(context.Background(), BroadcastParams{
		Type:       TypeAnnouncement,
		TargetType: TargetAnnouncement,
		TargetID:   7,
		Message:    "新しいお知らせ",
	}); err != nil {
		t.Fatalf("PublishToAllActiveUsers returned an error: %v", err)
	}

	if repo.broadcastCalls != 1 {
		t.Fatalf("expected exactly 1 broadcast insert, got %d", repo.broadcastCalls)
	}
	if repo.broadcastParam.Message != "新しいお知らせ" {
		t.Fatalf("unexpected message stored: %q", repo.broadcastParam.Message)
	}
	if repo.broadcastParam.TargetID == nil || *repo.broadcastParam.TargetID != 7 {
		t.Fatalf("unexpected target id stored: %v", repo.broadcastParam.TargetID)
	}
	// 接続者がいなければ配信のための引き直しもしない（往復は1回だけ）。
	if repo.lookupCalls != 0 {
		t.Fatalf("expected no delivery lookup with nobody connected, got %d", repo.lookupCalls)
	}
	if len(delivery.delivered) != 0 {
		t.Fatalf("expected no deliveries with nobody connected, got %v", delivery.delivered)
	}
}

// 配信の宛先は「接続している人」だけ。接続していない人のぶんを送ると、
// 使われない履歴が Broker に積み上がる（切断中のぶんは再接続時に取り直される）。
func TestPublishToAllActiveUsers_DeliversOnlyToConnectedUsers(t *testing.T) {
	repo := &countingNotificationRepo{rows: announcementRows(1, 2, 3, 4, 5)}
	delivery := &recordingDelivery{connected: []int64{2, 4}}
	p := NewNotificationPublisher(repo, delivery)

	if err := p.PublishToAllActiveUsers(context.Background(), BroadcastParams{
		Type:       TypeAnnouncement,
		TargetType: TargetAnnouncement,
		TargetID:   7,
		Message:    "新しいお知らせ",
	}); err != nil {
		t.Fatalf("PublishToAllActiveUsers returned an error: %v", err)
	}

	// 保存1本 + 接続者ぶんの引き直し1本 = 往復は利用者数に依らず2回。
	if repo.broadcastCalls != 1 || repo.lookupCalls != 1 {
		t.Fatalf("expected 1 insert and 1 lookup, got %d and %d", repo.broadcastCalls, repo.lookupCalls)
	}
	if len(repo.lookupUserIDs) != 2 {
		t.Fatalf("lookup should be scoped to the connected users, got %v", repo.lookupUserIDs)
	}

	if len(delivery.delivered) != 2 {
		t.Fatalf("expected 2 deliveries, got %v", delivery.delivered)
	}
	for _, userID := range delivery.delivered {
		if userID != 2 && userID != 4 {
			t.Fatalf("delivered to a user who is not connected: %d", userID)
		}
	}
	// イベント名は従来どおり notification（お知らせはトーストを出す通知なので、
	// 事実だけを送る notifications_changed には寄せていない）。
	for _, ev := range delivery.events {
		if ev != "notification" {
			t.Fatalf("unexpected event type %q", ev)
		}
	}
}
