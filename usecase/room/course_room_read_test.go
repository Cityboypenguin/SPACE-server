package room

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeCourseRoomReadRepo struct {
	repository.CourseRoomReadRepository
	position *repository.ReadPosition

	upsertedRoomID, upsertedUserID, upsertedReadAt int64
	upsertedMessageID                              *int64
}

func (f *fakeCourseRoomReadRepo) UpsertLastRead(_ context.Context, roomID, userID int64, lastReadMessageID *int64, readAt int64) error {
	f.upsertedRoomID, f.upsertedUserID, f.upsertedReadAt = roomID, userID, readAt
	f.upsertedMessageID = lastReadMessageID
	return nil
}

func (f *fakeCourseRoomReadRepo) GetLastRead(_ context.Context, _, _ int64) (*repository.ReadPosition, error) {
	return f.position, nil
}

type fakeMessageRepoForUnread struct {
	repository.MessageUnreadCounter
	gotOrigin repository.UnreadOrigin
	count     int
}

func (f *fakeMessageRepoForUnread) CountUnreadMessages(_ context.Context, _, _ int64, origin repository.UnreadOrigin) (int, error) {
	f.gotOrigin = origin
	return f.count, nil
}

// fakeMessageReaderForRead は既読位置として保存される「ルームの最新メッセージID」を返す。
type fakeMessageReaderForRead struct {
	repository.MessageReadModel
	latestID *int64
	gotRoom  int64
}

func (f *fakeMessageReaderForRead) GetLatestMessageID(_ context.Context, roomID int64) (*int64, error) {
	f.gotRoom = roomID
	return f.latestID, nil
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

func int64Ptr(v int64) *int64 { return &v }

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

// 既読位置は「そのルームの最新メッセージID」をサーバ側で解決して保存する。
// クライアントから ID を受け取る形にすると GraphQL スキーマの変更が要るため。
func TestMarkCourseRoomAsRead_RecordsLatestMessageIDAsReadPosition(t *testing.T) {
	repo := &fakeCourseRoomReadRepo{}
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(42)}
	uc := NewMarkCourseRoomAsReadUseCase(repo, reader)

	if err := uc.Execute(context.Background(), 5, 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.upsertedRoomID != 5 || repo.upsertedUserID != 7 || repo.upsertedReadAt == 0 {
		t.Fatalf("UpsertLastRead(room=%d, user=%d, readAt=%d), want room=5 user=7 with a timestamp",
			repo.upsertedRoomID, repo.upsertedUserID, repo.upsertedReadAt)
	}
	if reader.gotRoom != 5 {
		t.Errorf("latest message ID was looked up for room %d, want 5", reader.gotRoom)
	}
	if repo.upsertedMessageID == nil || *repo.upsertedMessageID != 42 {
		t.Fatalf("stored read position = %v, want the room's latest message ID 42", repo.upsertedMessageID)
	}
}

// メッセージが1件も無いルームを既読にしても失敗させない（位置は nil のまま時刻だけ進む）。
func TestMarkCourseRoomAsRead_EmptyRoomStoresNoMessageID(t *testing.T) {
	repo := &fakeCourseRoomReadRepo{}
	uc := NewMarkCourseRoomAsReadUseCase(repo, &fakeMessageReaderForRead{})

	if err := uc.Execute(context.Background(), 5, 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.upsertedMessageID != nil {
		t.Fatalf("stored read position = %v, want nil for a room with no messages", *repo.upsertedMessageID)
	}
}

// 既読位置にメッセージIDがあるときは、そちらが起点。時刻は使わない
// （同じ秒に既読更新と新着が起きても未読が漏れないのはこの優先順位のおかげ）。
func TestGetCourseRoomReadStatus_PrefersMessageIDOverTimestamp(t *testing.T) {
	readAt := int64(1700000000)
	registeredAt := int64(1600000000)
	messageRepo := &fakeMessageRepoForUnread{count: 3}
	position := &repository.ReadPosition{LastReadMessageID: int64Ptr(42), LastReadAt: &readAt}
	uc := newCourseRoomReadStatusUseCase(&fakeCourseRoomReadRepo{position: position}, messageRepo, &registeredAt)

	status, err := uc.Execute(context.Background(), 3, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if messageRepo.gotOrigin.LastReadMessageID == nil || *messageRepo.gotOrigin.LastReadMessageID != 42 {
		t.Fatalf("unread origin = %+v, want the last read message ID 42", messageRepo.gotOrigin)
	}
	// 表示用の lastReadAt は従来どおり返す（GraphQL の契約を変えない）。
	if status.LastReadAt == nil || *status.LastReadAt != readAt || status.UnreadCount != 3 {
		t.Fatalf("status = %+v, want lastReadAt=%d unread=3", status, readAt)
	}
	if status.PartnerLastReadAt != nil {
		t.Fatal("course rooms must not expose other users' read positions")
	}
}

// 列が追加される前からある既読行（message ID が NULL）は、従来どおり時刻起点。
func TestGetCourseRoomReadStatus_FallsBackToTimestampWhenMessageIDIsNull(t *testing.T) {
	readAt := int64(1700000000)
	registeredAt := int64(1600000000)
	messageRepo := &fakeMessageRepoForUnread{count: 3}
	position := &repository.ReadPosition{LastReadAt: &readAt}
	uc := newCourseRoomReadStatusUseCase(&fakeCourseRoomReadRepo{position: position}, messageRepo, &registeredAt)

	if _, err := uc.Execute(context.Background(), 3, 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	origin := messageRepo.gotOrigin
	if origin.LastReadMessageID != nil {
		t.Fatalf("unread origin = %+v, want no message ID", origin)
	}
	if origin.LastReadAt == nil || *origin.LastReadAt != readAt {
		t.Fatalf("unread origin = %+v, want the last read timestamp %d", origin, readAt)
	}
	// 既読位置があるので、時間割の登録時刻は起点にしない。
	if origin.FallbackAt != nil {
		t.Fatalf("unread origin fallback = %v, want none while a read position exists", *origin.FallbackAt)
	}
}

// まだ一度も開いていない授業は、授業一覧のバッジ（CountUnreadByCourseRooms）と同じく
// 時間割の登録時刻を起点にする。一覧と部屋内で未読数が食い違わないことがこのテストの主眼。
func TestGetCourseRoomReadStatus_NeverReadUsesTimetableRegistrationAsOrigin(t *testing.T) {
	registeredAt := int64(1600000000)
	messageRepo := &fakeMessageRepoForUnread{count: 2}
	uc := newCourseRoomReadStatusUseCase(&fakeCourseRoomReadRepo{}, messageRepo, &registeredAt)

	status, err := uc.Execute(context.Background(), 3, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	origin := messageRepo.gotOrigin
	if origin.LastReadMessageID != nil || origin.LastReadAt != nil {
		t.Fatalf("unread origin = %+v, want no read position", origin)
	}
	if origin.FallbackAt == nil || *origin.FallbackAt != registeredAt {
		t.Fatalf("unread origin fallback = %+v, want timetable registration %d", origin.FallbackAt, registeredAt)
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
	origin := messageRepo.gotOrigin
	if origin.LastReadMessageID != nil || origin.LastReadAt != nil || origin.FallbackAt != nil {
		t.Fatalf("unread origin = %+v, want nothing (every message unread)", origin)
	}
	if status.LastReadAt != nil || status.UnreadCount != 10 {
		t.Fatalf("status = %+v, want no read position and every message unread", status)
	}
}
