package room

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeIdentityRepo struct {
	repository.RoomAnonymousIdentityRepository
	lastReadAt *int64

	upsertedRoomID, upsertedUserID, upsertedReadAt int64
}

func (f *fakeIdentityRepo) UpsertLastReadAt(_ context.Context, roomID, userID, readAt int64) error {
	f.upsertedRoomID, f.upsertedUserID, f.upsertedReadAt = roomID, userID, readAt
	return nil
}

func (f *fakeIdentityRepo) GetLastReadAt(_ context.Context, _, _ int64) (*int64, error) {
	return f.lastReadAt, nil
}

type fakeMessageRepoForUnread struct {
	repository.MessageRepository
	gotAfter int64
	count    int
}

func (f *fakeMessageRepoForUnread) CountUnreadMessages(_ context.Context, _, _ int64, afterTimestamp int64) (int, error) {
	f.gotAfter = afterTimestamp
	return f.count, nil
}

func TestMarkCourseRoomAsRead_RecordsReadPosition(t *testing.T) {
	repo := &fakeIdentityRepo{}
	uc := NewMarkCourseRoomAsReadUseCase(repo)

	if err := uc.Execute(context.Background(), 5, 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.upsertedRoomID != 5 || repo.upsertedUserID != 7 || repo.upsertedReadAt == 0 {
		t.Fatalf("UpsertLastReadAt(room=%d, user=%d, readAt=%d), want room=5 user=7 with a timestamp",
			repo.upsertedRoomID, repo.upsertedUserID, repo.upsertedReadAt)
	}
}

func TestGetCourseRoomReadStatus_CountsMessagesAfterReadPosition(t *testing.T) {
	readAt := int64(1700000000)
	messageRepo := &fakeMessageRepoForUnread{count: 3}
	uc := NewGetCourseRoomReadStatusUseCase(&fakeIdentityRepo{lastReadAt: &readAt}, messageRepo)

	status, err := uc.Execute(context.Background(), 5, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if messageRepo.gotAfter != readAt {
		t.Fatalf("CountUnreadMessages after = %d, want %d", messageRepo.gotAfter, readAt)
	}
	if status.LastReadAt == nil || *status.LastReadAt != readAt || status.UnreadCount != 3 {
		t.Fatalf("status = %+v, want lastReadAt=%d unread=3", status, readAt)
	}
	if status.PartnerLastReadAt != nil {
		t.Fatal("course rooms must not expose other users' read positions")
	}
}

func TestGetCourseRoomReadStatus_NeverRead(t *testing.T) {
	messageRepo := &fakeMessageRepoForUnread{count: 10}
	uc := NewGetCourseRoomReadStatusUseCase(&fakeIdentityRepo{}, messageRepo)

	status, err := uc.Execute(context.Background(), 5, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.LastReadAt != nil || messageRepo.gotAfter != 0 || status.UnreadCount != 10 {
		t.Fatalf("status = %+v (after=%d), want no read position and every message unread", status, messageRepo.gotAfter)
	}
}
