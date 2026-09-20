package room

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeRoomUserRepoForRead struct {
	repository.RoomUserRepository
	position *repository.ReadPosition
	// members は GetMembersLastReadAt が返すメンバーごとの既読時刻。
	members map[int64]*int64
	// membersCalls は「相手の既読を引きに行ったか」。DM 以外では引かないことを確かめる。
	membersCalls int

	updatedRoomID, updatedUserID, updatedReadAt int64
	updatedMessageID                            *int64
	// updateCalls は既読位置を書きに行った回数。検証に落ちた要求で1行も書かないことの確認に使う。
	updateCalls int
}

func (f *fakeRoomUserRepoForRead) UpdateLastRead(_ context.Context, roomID, userID int64, lastReadMessageID *int64, readAt int64) error {
	f.updateCalls++
	f.updatedRoomID, f.updatedUserID, f.updatedReadAt = roomID, userID, readAt
	f.updatedMessageID = lastReadMessageID
	return nil
}

func (f *fakeRoomUserRepoForRead) GetLastRead(_ context.Context, _, _ int64) (*repository.ReadPosition, error) {
	return f.position, nil
}

func (f *fakeRoomUserRepoForRead) GetMembersLastReadAt(_ context.Context, _ int64) (map[int64]*int64, error) {
	f.membersCalls++
	if f.members == nil {
		return map[int64]*int64{}, nil
	}
	return f.members, nil
}

func communityRoom(id int64) *model.Room {
	return &model.Room{ID: id, Type: model.RoomTypeCommunity}
}

func dmRoom(id int64) *model.Room {
	return &model.Room{ID: id, Type: model.RoomTypeDM}
}

// DM・コミュニティの既読位置も授業ルームと同じくメッセージIDで持つ。
// 片方だけ時刻のままにすると未読判定が2種類に分かれるので、機構をひとつに揃えている。
func TestMarkRoomAsRead_RecordsLatestMessageIDAsReadPosition(t *testing.T) {
	repo := &fakeRoomUserRepoForRead{}
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(77)}
	uc := NewMarkRoomAsReadUseCase(repo, reader)

	if err := uc.Execute(context.Background(), 4, 8, nil); err != nil {
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

	status, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), communityRoom(4), 8)
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

	if _, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), communityRoom(4), 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter.gotOrigin.LastReadMessageID != nil {
		t.Fatalf("unread origin = %+v, want no message ID", counter.gotOrigin)
	}
	if counter.gotOrigin.LastReadAt == nil || *counter.gotOrigin.LastReadAt != readAt {
		t.Fatalf("unread origin = %+v, want the last read timestamp %d", counter.gotOrigin, readAt)
	}

	neverRead := &fakeRoomUserRepoForRead{}
	if _, err := NewGetRoomReadStatusUseCase(neverRead, counter).Execute(context.Background(), communityRoom(4), 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	origin := counter.gotOrigin
	if origin.LastReadMessageID != nil || origin.LastReadAt != nil || origin.FallbackAt != nil {
		t.Fatalf("unread origin = %+v, want nothing (every message unread)", origin)
	}
}

// fakeRegistrantLister は timetables 起点の宛先取得。room_users 起点のものと
// 取り違えていないかを見るために、引数をそのまま覚えておく。
type fakeRegistrantLister struct {
	repository.TimetableRepository
	gotRoomID int64
	ids       []int64
}

func (f *fakeRegistrantLister) ListRegistrantIDsByCourseRoomID(_ context.Context, roomID int64) ([]int64, error) {
	f.gotRoomID = roomID
	return f.ids, nil
}

// 授業ルームの更新通知の宛先は履修者（timetables）であって room_users ではない。
// room_users を起点にすると授業ルームでは宛先が空になり、更新が誰にも届かない。
//
// 返すのはIDだけで未読数は含まない（未読数は受け取った側が自分ぶんだけ取り直す）。
func TestGetCourseRegistrantIDs_ListsRegistrantsOfTheCourseRoom(t *testing.T) {
	lister := &fakeRegistrantLister{ids: []int64{10, 11, 12}}

	ids, err := NewGetCourseRegistrantIDsUseCase(lister).Execute(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lister.gotRoomID != 3 {
		t.Fatalf("looked up room=%d, want room=3", lister.gotRoomID)
	}
	if len(ids) != 3 || ids[0] != 10 || ids[1] != 11 || ids[2] != 12 {
		t.Fatalf("ids = %v, want every registrant of the room", ids)
	}
}

// --- 相手の既読位置は DM 限定 ---------------------------------------------

// DM では「自分以外のちょうど1人」が相手なので、その既読時刻を返す。
func TestGetRoomReadStatus_DMReturnsThePartnersLastRead(t *testing.T) {
	partnerReadAt := int64(1700000500)
	repo := &fakeRoomUserRepoForRead{members: map[int64]*int64{8: nil, 9: &partnerReadAt}}
	counter := &fakeMessageRepoForUnread{}

	status, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), dmRoom(4), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.PartnerLastReadAt == nil || *status.PartnerLastReadAt != partnerReadAt {
		t.Fatalf("partnerLastReadAt = %v, want the other member's %d", status.PartnerLastReadAt, partnerReadAt)
	}
}

// コミュニティで「自分以外の誰か1人」の既読を返すと、map の走査順が不定なので
// 呼ぶたびに別人の既読時刻になる。誰のものか説明できない値は返さない。
func TestGetRoomReadStatus_CommunityNeverReturnsAPartnersLastRead(t *testing.T) {
	a, b := int64(1700000100), int64(1700000200)
	repo := &fakeRoomUserRepoForRead{members: map[int64]*int64{8: nil, 9: &a, 10: &b}}
	counter := &fakeMessageRepoForUnread{}

	status, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), communityRoom(4), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.PartnerLastReadAt != nil {
		t.Fatalf("partnerLastReadAt = %v, want nil outside DMs", *status.PartnerLastReadAt)
	}
	if repo.membersCalls != 0 {
		t.Fatalf("GetMembersLastReadAt calls = %d, want 0 (使わない値のためにクエリを増やさない)", repo.membersCalls)
	}
}

// 相手が退会して room_users の行が消えた DM では相手が決まらない。
// 適当な1人（＝自分）を拾わず、「分からない」として nil を返す。
func TestGetRoomReadStatus_DMWithoutAPartnerReturnsNil(t *testing.T) {
	mine := int64(1700000300)
	repo := &fakeRoomUserRepoForRead{members: map[int64]*int64{8: &mine}}
	counter := &fakeMessageRepoForUnread{}

	status, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), dmRoom(4), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.PartnerLastReadAt != nil {
		t.Fatalf("partnerLastReadAt = %v, want nil when the partner is gone", *status.PartnerLastReadAt)
	}
}

func TestGetRoomReadStatusBatch_PartnerLastReadOnlyForDMs(t *testing.T) {
	partnerReadAt := int64(1700000500)
	repo := &fakeRoomUserBatchRepo{members: map[int64]map[int64]*int64{
		4: {8: nil, 9: &partnerReadAt},
	}}
	counter := &fakeBatchUnreadCounter{}

	dmStatuses, err := NewGetRoomReadStatusBatchUseCase(repo, counter).
		Execute(context.Background(), []int64{4}, 8, model.RoomTypeDM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := dmStatuses[4].PartnerLastReadAt; got == nil || *got != partnerReadAt {
		t.Fatalf("partnerLastReadAt = %v, want %d for a DM", got, partnerReadAt)
	}

	repo.membersCalls = 0
	communityStatuses, err := NewGetRoomReadStatusBatchUseCase(repo, counter).
		Execute(context.Background(), []int64{4}, 8, model.RoomTypeCommunity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := communityStatuses[4].PartnerLastReadAt; got != nil {
		t.Fatalf("partnerLastReadAt = %v, want nil for a community", *got)
	}
	if repo.membersCalls != 0 {
		t.Fatalf("GetMembersLastReadAtByRoomIDs calls = %d, want 0 for communities", repo.membersCalls)
	}
}

type fakeBatchUnreadCounter struct {
	repository.MessageUnreadCounter
}

func (f *fakeBatchUnreadCounter) CountUnreadMessagesByRoomIDs(_ context.Context, _ int64, _ []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

type fakeRoomUserBatchRepo struct {
	repository.RoomUserRepository
	members      map[int64]map[int64]*int64
	membersCalls int
	positions    map[int64]*repository.ReadPosition
}

func (f *fakeRoomUserBatchRepo) GetLastReadByRoomIDs(_ context.Context, _ int64, _ []int64) (map[int64]*repository.ReadPosition, error) {
	return f.positions, nil
}

func (f *fakeRoomUserBatchRepo) GetMembersLastReadAtByRoomIDs(_ context.Context, _ []int64) (map[int64]map[int64]*int64, error) {
	f.membersCalls++
	return f.members, nil
}

// --- クライアントが指定してきた既読位置 -------------------------------------

// 既読位置には「クライアントが実際に画面へ出した最後のメッセージ」を使う。
// サーバが受信時点の最新メッセージを採ると、クライアントがまだ描画していない新着まで
// 既読になり、そのぶんが未読から永久に落ちる（この取りこぼしを塞ぐのが引数の目的）。
func TestMarkRoomAsRead_UsesTheRequestedMessageIDInsteadOfTheRoomsLatest(t *testing.T) {
	repo := &fakeRoomUserRepoForRead{}
	// ルームの最新は 101 だが、クライアントが出したのは 100 まで。
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(101), messagesInRoom: map[int64]bool{100: true, 101: true}}
	uc := NewMarkRoomAsReadUseCase(repo, reader)

	if err := uc.Execute(context.Background(), 4, 8, int64Ptr(100)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedMessageID == nil || *repo.updatedMessageID != 100 {
		t.Fatalf("stored read position = %v, want the displayed message 100", repo.updatedMessageID)
	}
	if reader.latestCalls != 0 {
		t.Errorf("latest message ID lookups = %d, want 0 when the client told us its position", reader.latestCalls)
	}
	if reader.existsCalls != 1 {
		t.Errorf("validation lookups = %d, want 1", reader.existsCalls)
	}
}

// 他ルームのメッセージIDは拒否する。未読判定は m.id > last_read_message_id という
// 単純比較なので、他ルームの大きいIDを1回渡せばそのルームの未読を丸ごと消せてしまう。
func TestMarkRoomAsRead_RejectsAMessageIDFromAnotherRoom(t *testing.T) {
	repo := &fakeRoomUserRepoForRead{}
	// room 4 には 100 しか無い。999 は別ルーム（または存在しない）メッセージ。
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(100), messagesInRoom: map[int64]bool{100: true}}
	uc := NewMarkRoomAsReadUseCase(repo, reader)

	err := uc.Execute(context.Background(), 4, 8, int64Ptr(999))
	if err == nil {
		t.Fatal("expected a message ID from another room to be rejected")
	}
	if code := apperr.CodeOf(err); code != apperr.CodeInvalidInput {
		t.Errorf("error code = %q, want %q", code, apperr.CodeInvalidInput)
	}
	// 拒否したときは既読時刻も含めて1行も書かない（壊れた要求で位置が動かないこと）。
	if repo.updateCalls != 0 {
		t.Errorf("read position writes = %d, want 0 for a rejected request", repo.updateCalls)
	}
}

// 授業ルーム（course_room_reads 側）でも検証は同じ。片方だけ素通しにしない。
func TestMarkCourseRoomAsRead_RejectsAMessageIDFromAnotherRoom(t *testing.T) {
	repo := &fakeCourseRoomReadRepo{}
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(42), messagesInRoom: map[int64]bool{42: true}}
	uc := NewMarkCourseRoomAsReadUseCase(repo, reader)

	err := uc.Execute(context.Background(), 5, 7, int64Ptr(999))
	if err == nil {
		t.Fatal("expected a message ID from another room to be rejected")
	}
	if code := apperr.CodeOf(err); code != apperr.CodeInvalidInput {
		t.Errorf("error code = %q, want %q", code, apperr.CodeInvalidInput)
	}
	if repo.upsertedRoomID != 0 || repo.upsertedMessageID != nil {
		t.Errorf("read position was written (%+v) for a rejected request", repo)
	}
}

// 引数を渡してこない古いクライアントは従来動作（受信時点の最新メッセージが既読位置）。
// 検証クエリも走らせない（渡ってきていない値を検証しても意味がない）。
func TestMarkRoomAsRead_WithoutARequestedIDKeepsTheLegacyBehaviour(t *testing.T) {
	repo := &fakeRoomUserRepoForRead{}
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(101)}
	uc := NewMarkRoomAsReadUseCase(repo, reader)

	if err := uc.Execute(context.Background(), 4, 8, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedMessageID == nil || *repo.updatedMessageID != 101 {
		t.Fatalf("stored read position = %v, want the room's latest message 101", repo.updatedMessageID)
	}
	if reader.existsCalls != 0 {
		t.Errorf("validation lookups = %d, want 0 when no position was supplied", reader.existsCalls)
	}
}

// 既読位置は巻き戻さない。
//
// ユースケースは「進めたい位置」を決めるだけで、現在位置との比較はしない（複数端末が
// 同時に既読を打つので、読んで比べて書くと競合する）。実際の巻き戻し防止はリポジトリの
// UPDATE 文が GREATEST で行い、その効き方は infra/mysql の
// TestUpdateLastRead_NeverMovesBackwards が本物の MySQL で確かめている。
// ここで固定したいのは「古いIDが来ても途中で握りつぶさず、そのまま SQL へ渡す」こと
// （ユースケース側でクランプを足すと、2箇所で巻き戻し防止を持つことになる）。
func TestMarkRoomAsRead_LeavesRewindProtectionToTheRepository(t *testing.T) {
	repo := &fakeRoomUserRepoForRead{}
	reader := &fakeMessageReaderForRead{latestID: int64Ptr(101), messagesInRoom: map[int64]bool{50: true, 101: true}}
	uc := NewMarkRoomAsReadUseCase(repo, reader)

	if err := uc.Execute(context.Background(), 4, 8, int64Ptr(50)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedMessageID == nil || *repo.updatedMessageID != 50 {
		t.Fatalf("stored read position = %v, want the requested 50 passed through untouched", repo.updatedMessageID)
	}
}

// 既読位置のメッセージIDは未読件数と同じ起点なので、必ず一緒に返す。
// これが欠けるとクライアントは時刻（秒）でしか未読ページを取れず、
// 「未読件数は1件なのに未読ページは0件」という食い違いが戻ってくる。
func TestGetRoomReadStatus_ReturnsTheReadPositionMessageID(t *testing.T) {
	readAt := int64(1700000000)
	counter := &fakeMessageRepoForUnread{count: 1}
	repo := &fakeRoomUserRepoForRead{position: &repository.ReadPosition{LastReadMessageID: int64Ptr(77), LastReadAt: &readAt}}

	status, err := NewGetRoomReadStatusUseCase(repo, counter).Execute(context.Background(), communityRoom(4), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.LastReadMessageID == nil || *status.LastReadMessageID != 77 {
		t.Fatalf("lastReadMessageID = %v, want 77 (未読件数と同じ起点を返す)", status.LastReadMessageID)
	}
}

// 一覧（DM・コミュニティ）でも既読位置のメッセージIDを返す。一覧から開いたときだけ
// null になると、未読ページの起点が経路によって ID と時刻で揺れる。
func TestGetRoomReadStatusBatch_ReturnsTheReadPositionMessageID(t *testing.T) {
	readAt := int64(1700000000)
	repo := &fakeRoomUserBatchRepo{positions: map[int64]*repository.ReadPosition{
		4: {LastReadMessageID: int64Ptr(77), LastReadAt: &readAt},
	}}
	counter := &fakeBatchUnreadCounter{}

	statuses, err := NewGetRoomReadStatusBatchUseCase(repo, counter).
		Execute(context.Background(), []int64{4, 5}, 8, model.RoomTypeDM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := statuses[4].LastReadMessageID; got == nil || *got != 77 {
		t.Fatalf("lastReadMessageID = %v, want 77", got)
	}
	// 一度も読んでいないルームは位置なし（0 ではなく nil）。
	if got := statuses[5].LastReadMessageID; got != nil {
		t.Fatalf("lastReadMessageID = %v, want nil for a room that was never read", *got)
	}
}
