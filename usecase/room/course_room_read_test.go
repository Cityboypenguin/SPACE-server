package room

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeCourseRoomReadRepo struct {
	repository.CourseRoomReadRepository
	lastReadAt *int64

	upsertedRoomID, upsertedUserID, upsertedReadAt int64
}

func (f *fakeCourseRoomReadRepo) UpsertLastReadAt(_ context.Context, roomID, userID, readAt int64) error {
	f.upsertedRoomID, f.upsertedUserID, f.upsertedReadAt = roomID, userID, readAt
	return nil
}

func (f *fakeCourseRoomReadRepo) GetLastReadAt(_ context.Context, _, _ int64) (*int64, error) {
	return f.lastReadAt, nil
}

type fakeMessageRepoForUnread struct {
	repository.MessageUnreadCounter
	gotAfter int64
	count    int
}

func (f *fakeMessageRepoForUnread) CountUnreadMessages(_ context.Context, _, _ int64, afterTimestamp int64) (int, error) {
	f.gotAfter = afterTimestamp
	return f.count, nil
}

type fakeCourseRepoForUnread struct {
	repository.CourseRepository
	course *model.Course
}

func (f *fakeCourseRepoForUnread) GetCourseByRoomID(_ context.Context, _ int64) (*model.Course, error) {
	return f.course, nil
}

type fakeTimetableRepoForUnread struct {
	repository.TimetableRepository
	registeredAt *int64
}

func (f *fakeTimetableRepoForUnread) GetRegisteredAt(_ context.Context, _, _ int64) (*int64, error) {
	return f.registeredAt, nil
}

// newCourseRoomReadStatusUseCase は「room 3 は course 9 のチャット」という前提で組む。
func newCourseRoomReadStatusUseCase(
	readRepo *fakeCourseRoomReadRepo,
	messageRepo *fakeMessageRepoForUnread,
	registeredAt *int64,
) GetCourseRoomReadStatusUseCase {
	return NewGetCourseRoomReadStatusUseCase(
		readRepo,
		messageRepo,
		&fakeCourseRepoForUnread{course: &model.Course{ID: 9, RoomID: 3}},
		&fakeTimetableRepoForUnread{registeredAt: registeredAt},
	)
}

func TestMarkCourseRoomAsRead_RecordsReadPosition(t *testing.T) {
	repo := &fakeCourseRoomReadRepo{}
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
	registeredAt := int64(1600000000)
	messageRepo := &fakeMessageRepoForUnread{count: 3}
	uc := newCourseRoomReadStatusUseCase(&fakeCourseRoomReadRepo{lastReadAt: &readAt}, messageRepo, &registeredAt)

	status, err := uc.Execute(context.Background(), 3, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 既読位置があるときは、時間割の登録時刻ではなくそちらが起点。
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

// まだ一度も開いていない授業は、授業一覧のバッジ（CountUnreadByCourseRooms の
// COALESCE(last_read_at, timetables.created_at)）と同じく時間割の登録時刻を起点にする。
// 一覧と部屋内で未読数が食い違わないことがこのテストの主眼。
func TestGetCourseRoomReadStatus_NeverReadUsesTimetableRegistrationAsOrigin(t *testing.T) {
	registeredAt := int64(1600000000)
	messageRepo := &fakeMessageRepoForUnread{count: 2}
	uc := newCourseRoomReadStatusUseCase(&fakeCourseRoomReadRepo{}, messageRepo, &registeredAt)

	status, err := uc.Execute(context.Background(), 3, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if messageRepo.gotAfter != registeredAt {
		t.Fatalf("CountUnreadMessages after = %d, want timetable registration %d", messageRepo.gotAfter, registeredAt)
	}
	if status.LastReadAt != nil || status.UnreadCount != 2 {
		t.Fatalf("status = %+v, want no read position and unread=2", status)
	}
}

// 時間割に登録していない（読むだけの）ユーザーには起点が無いので全件カウント。
// この場合は授業一覧のバッジにも出てこないため、食い違いは起きない。
func TestGetCourseRoomReadStatus_NeverReadAndNotRegistered(t *testing.T) {
	messageRepo := &fakeMessageRepoForUnread{count: 10}
	uc := newCourseRoomReadStatusUseCase(&fakeCourseRoomReadRepo{}, messageRepo, nil)

	status, err := uc.Execute(context.Background(), 3, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.LastReadAt != nil || messageRepo.gotAfter != 0 || status.UnreadCount != 10 {
		t.Fatalf("status = %+v (after=%d), want no read position and every message unread", status, messageRepo.gotAfter)
	}
}
