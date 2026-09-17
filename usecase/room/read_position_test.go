package room

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeRoomUserRepoForRead struct {
	repository.RoomUserRepository
	position *repository.ReadPosition

	updatedRoomID, updatedUserID, updatedReadAt int64
	updatedMessageID                            *int64
}

func (f *fakeRoomUserRepoForRead) UpdateLastRead(_ context.Context, roomID, userID int64, lastReadMessageID *int64, readAt int64) error {
	f.updatedRoomID, f.updatedUserID, f.updatedReadAt = roomID, userID, readAt
	f.updatedMessageID = lastReadMessageID
	return nil
}

func (f *fakeRoomUserRepoForRead) GetLastRead(_ context.Context, _, _ int64) (*repository.ReadPosition, error) {
	return f.position, nil
}

func (f *fakeRoomUserRepoForRead) GetMembersLastReadAt(_ context.Context, _ int64) (map[int64]*int64, error) {
	return map[int64]*int64{}, nil
}

// DM・コミュニティの既読位置も授業ルームと同じくメッセージIDで持つ。
// 片方だけ時刻のままにすると未読判定が2種類に分かれるので、機構をひとつに揃えている。
func TestMarkRoomAsRead_RecordsLatestMessageIDAsReadPosition(t *testing.T) {
	repo := &fakeRoomUserRepoForRead{}
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(77)}
	uc := NewMarkRoomAsReadUseCase(repo, reader)

	if err := uc.Execute(context.Background(), 4, 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedRoomID != 4 || repo.updatedUserID != 8 || repo.updatedReadAt == 0 {
		t.Fatalf("UpdateLastRead(room=%d, user=%d, readAt=%d), want room=4 user=8 with a timestamp",
			repo.updatedRoomID, repo.updatedUserID, repo.updatedReadAt)
	}
	if repo.updatedMessageID == nil || *repo.updatedMessageID != 77 {
		t.Fatalf("stored read position = %v, want the room's latest message ID 77", repo.updatedMessageID)
	}
}

func TestGetRoomReadStatus_PrefersMessageIDOverTimestamp(t *testing.T) {
	readAt := int64(1700000000)
	counter := &fakeMessageRepoForUnread{count: 5}
	repo := &fakeRoomUserRepoForRead{position: &repository.ReadPosition{LastReadMessageID: int64Ptr(77), LastReadAt: &readAt}}

	status, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), 4, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter.gotOrigin.LastReadMessageID == nil || *counter.gotOrigin.LastReadMessageID != 77 {
		t.Fatalf("unread origin = %+v, want the last read message ID 77", counter.gotOrigin)
	}
	if status.LastReadAt == nil || *status.LastReadAt != readAt || status.UnreadCount != 5 {
		t.Fatalf("status = %+v, want lastReadAt=%d unread=5", status, readAt)
	}
}

// 本番稼働中の room_users に列を足した直後の行（message ID が NULL）は時刻起点。
// 通常ルームはフォールバックの時刻を持たない（＝未読なら全件）。
func TestGetRoomReadStatus_FallsBackToTimestampThenCountsEverything(t *testing.T) {
	readAt := int64(1700000000)
	counter := &fakeMessageRepoForUnread{}
	repo := &fakeRoomUserRepoForRead{position: &repository.ReadPosition{LastReadAt: &readAt}}

	if _, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), 4, 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter.gotOrigin.LastReadMessageID != nil {
		t.Fatalf("unread origin = %+v, want no message ID", counter.gotOrigin)
	}
	if counter.gotOrigin.LastReadAt == nil || *counter.gotOrigin.LastReadAt != readAt {
		t.Fatalf("unread origin = %+v, want the last read timestamp %d", counter.gotOrigin, readAt)
	}

	neverRead := &fakeRoomUserRepoForRead{}
	if _, err := NewGetRoomReadStatusUseCase(neverRead, counter).Execute(context.Background(), 4, 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	origin := counter.gotOrigin
	if origin.LastReadMessageID != nil || origin.LastReadAt != nil || origin.FallbackAt != nil {
		t.Fatalf("unread origin = %+v, want nothing (every message unread)", origin)
	}
}

type fakeCourseRegistrantCounter struct {
	repository.MessageUnreadCounter
	perMemberCalled bool
	gotRoomID       int64
	gotExcludeID    int64
	counts          map[int64]int
}

func (f *fakeCourseRegistrantCounter) CountUnreadMessagesPerMember(_ context.Context, _ int64, _ int64) (map[int64]int, error) {
	f.perMemberCalled = true
	return map[int64]int{}, nil
}

func (f *fakeCourseRegistrantCounter) CountUnreadMessagesPerCourseRegistrant(_ context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	f.gotRoomID, f.gotExcludeID = roomID, excludeUserID
	return f.counts, nil
}

// 授業ルームの未読SSEの宛先は履修者（timetables）であって room_users ではない。
// room_users を起点に数えると授業ルームでは宛先が空になり、未読の更新が誰にも届かない。
func TestGetCourseRoomUnreadCounts_TargetsRegistrantsNotRoomUsers(t *testing.T) {
	counter := &fakeCourseRegistrantCounter{counts: map[int64]int{11: 2, 12: 5}}

	counts, err := NewGetCourseRoomUnreadCountsUseCase(counter).Execute(context.Background(), 3, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter.perMemberCalled {
		t.Fatal("course rooms must not be counted through room_users (CountUnreadMessagesPerMember)")
	}
	if counter.gotRoomID != 3 || counter.gotExcludeID != 10 {
		t.Fatalf("counted room=%d excluding user=%d, want room=3 excluding 10", counter.gotRoomID, counter.gotExcludeID)
	}
	if len(counts) != 2 || counts[11] != 2 || counts[12] != 5 {
		t.Fatalf("counts = %v, want every registrant except the sender", counts)
	}
}
