package chat

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// --- fakes -----------------------------------------------------------------
// 既存テスト (usecase/course/archive_test.go) と同じく、必要なメソッドだけ持つ
// 素の struct を使う。

type fakeGetRoom struct {
	rooms map[int64]*model.Room
}

func (f *fakeGetRoom) Execute(_ context.Context, id int64) (*model.Room, error) {
	room, ok := f.rooms[id]
	if !ok {
		return nil, fmt.Errorf("room not found: %d", id)
	}
	return room, nil
}

type fakeMembers struct {
	byRoom map[int64][]int64
	// calls は「権限判定のために membership を何回引いたか」。二重判定で DB
	// アクセスが倍になっていないことを確かめるのに使う。
	calls int
}

func (f *fakeMembers) Execute(_ context.Context, roomID int64) ([]int64, error) {
	f.calls++
	return f.byRoom[roomID], nil
}

type fakeRoomUserRole struct{ role string }

func (f *fakeRoomUserRole) Execute(_ context.Context, _, _ int64) (string, error) {
	return f.role, nil
}

type fakeCheckRoomWritable struct {
	err    error
	called bool
}

func (f *fakeCheckRoomWritable) Execute(_ context.Context, _ int64) error {
	f.called = true
	return f.err
}

type fakeCheckBlockRelation struct{ blocked bool }

func (f *fakeCheckBlockRelation) Execute(_ context.Context, _, _ int64) (bool, error) {
	return f.blocked, nil
}

type fakeGetMessage struct{ msg *model.Message }

func (f *fakeGetMessage) Execute(_ context.Context, _ int64) (*model.Message, error) {
	return f.msg, nil
}

type fakeSendMessage struct {
	savedMentions []*model.Mention
}

func (f *fakeSendMessage) Execute(_ context.Context, roomID, userID int64, content string, _ []model.MediaInput, replyToID *int64, mentions []*model.Mention) (*model.Message, error) {
	f.savedMentions = mentions
	return &model.Message{ID: 100, RoomID: roomID, UserID: userID, Content: content, ReplyToID: replyToID, Mentions: mentions}, nil
}

type fakeUpdateMessage struct{ called bool }

func (f *fakeUpdateMessage) Execute(_ context.Context, messageID int64, param model.UpdateMessageParam, mentions []*model.Mention) (*model.Message, error) {
	f.called = true
	content := ""
	if param.Content != nil {
		content = *param.Content
	}
	return &model.Message{ID: messageID, Content: content, Mentions: mentions}, nil
}

type fakeDeleteMessage struct{ called bool }

func (f *fakeDeleteMessage) Execute(_ context.Context, _, _ int64) (bool, error) {
	f.called = true
	return true, nil
}

// fakeResolveMentions は MentionsSupported と同じ仕様（コミュニティのみメンションが
// 成立する）を真似る。実体の検証は ResolveMentionsInteractor にあるが、サービスが
// 「解決済みの結果だけ」を保存に渡すことをここで確かめる。
type fakeResolveMentions struct {
	rooms map[int64]*model.Room
}

func (f *fakeResolveMentions) Execute(_ context.Context, roomID, _ int64, _ string, mentionUserIDs []int64) ([]*model.Mention, error) {
	if !messageusecase.MentionsSupported(f.rooms[roomID]) {
		return nil, nil
	}
	mentions := make([]*model.Mention, 0, len(mentionUserIDs))
	for _, id := range mentionUserIDs {
		mentions = append(mentions, &model.Mention{UserID: id, Text: "someone"})
	}
	return mentions, nil
}

type fakeListMentions struct {
	byMessage map[int64][]*model.Mention
	err       error
}

func (f *fakeListMentions) Execute(_ context.Context, _ []int64) (map[int64][]*model.Mention, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byMessage, nil
}

type fakeListMessageMedia struct {
	byMessage map[int64][]*model.Media
	err       error
}

func (f *fakeListMessageMedia) Execute(_ context.Context, _ []int64) (map[int64][]*model.Media, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byMessage, nil
}

// fakeListMessages / fakeListMessagesAround は一覧取得の代役。取得そのものではなく
// 「閲覧権限を通ってから取得しているか」を確かめるために呼ばれた回数だけ見る。
type fakeListMessages struct{ calls int }

func (f *fakeListMessages) Execute(_ context.Context, _ repository.MessageQuery) (*repository.MessagePage, error) {
	f.calls++
	return &repository.MessagePage{}, nil
}

type fakeListMessagesAround struct{ calls int }

func (f *fakeListMessagesAround) Execute(_ context.Context, _, _ int64, _ int) (*repository.MessagePage, error) {
	f.calls++
	return &repository.MessagePage{}, nil
}

// fakeEvents は配信・通知の出口を記録するだけの EventPublisher。
// 「何を送ったか」ではなく「送ったか送らなかったか」を確かめたい箇所で使う。
type fakeEvents struct {
	NoopEventPublisher
	updated []MessageUpdatedEvent
}

func (f *fakeEvents) MessageUpdated(_ context.Context, ev MessageUpdatedEvent) {
	f.updated = append(f.updated, ev)
}

type fakeAnonIdentity struct {
	calls  int
	roomID int64
}

func (f *fakeAnonIdentity) Execute(_ context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	f.calls++
	f.roomID = roomID
	return &model.RoomAnonymousIdentity{ID: 1, RoomID: roomID, UserID: userID, Label: "匿名001"}, nil
}

// fakeReadStatus は既読位置の取得（room_users 経路 / course_room_reads 経路）の代役。
// どちらが呼ばれたかで「ルーム種別で置き場を選べているか」を確かめる。
type fakeReadStatus struct {
	calls  int
	roomID int64
	status *roomusecase.RoomReadStatus
}

func (f *fakeReadStatus) Execute(_ context.Context, roomID, _ int64) (*roomusecase.RoomReadStatus, error) {
	f.calls++
	f.roomID = roomID
	return f.status, nil
}

type fakeMarkRead struct {
	calls  int
	roomID int64
}

func (f *fakeMarkRead) Execute(_ context.Context, roomID, _ int64) error {
	f.calls++
	f.roomID = roomID
	return nil
}

// --- helpers ---------------------------------------------------------------

const (
	courseRoomID    int64 = 1
	communityRoomID int64 = 2
	dmRoomID        int64 = 3
)

func testRooms() map[int64]*model.Room {
	return map[int64]*model.Room{
		courseRoomID:    {ID: courseRoomID, Type: model.RoomTypeCourse},
		communityRoomID: {ID: communityRoomID, Type: model.RoomTypeCommunity},
		dmRoomID:        {ID: dmRoomID, Type: model.RoomTypeDM},
	}
}

func ctxAsUser(id int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: id, Role: "user"})
}

func ctxAsAdmin(id int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: id, Role: "admin"})
}

// harness は4つのサービス（権限判定・送信/編集/削除・一覧・既読）を、判定を1つだけ
// 共有する形で組み立てる。本番の配線（cmd/server/main.go）と同じ形にしてあるので、
// 「テストでは通るが本番では別の判定が走る」が起きない。
type harness struct {
	accessDeps  AccessPolicyDeps
	commandDeps MessageCommandDeps
	queryDeps   MessageQueryDeps
	readDeps    ReadReceiptDeps

	access   AccessPolicy
	commands MessageCommandService
	queries  MessageQueryService
	reads    ReadReceiptService

	members       *fakeMembers
	readStatus    *fakeReadStatus
	courseRead    *fakeReadStatus
	markRead      *fakeMarkRead
	markCourse    *fakeMarkRead
	sendMessage   *fakeSendMessage
	updateMessage *fakeUpdateMessage
	deleteMessage *fakeDeleteMessage
	checkWritable *fakeCheckRoomWritable
	anonIdentity  *fakeAnonIdentity
	getMessage    *fakeGetMessage
	listMentions  *fakeListMentions
	listMedia     *fakeListMessageMedia
	listMessages  *fakeListMessages
	listAround    *fakeListMessagesAround
	events        *fakeEvents
}

// rebuild は deps を差し替えたあとにサービスを組み直す。
func (h *harness) rebuild() {
	h.access = NewAccessPolicy(h.accessDeps)
	h.commandDeps.Access = h.access
	h.queryDeps.Access = h.access
	h.readDeps.Access = h.access
	h.commands = NewMessageCommandService(h.commandDeps)
	h.queries = NewMessageQueryService(h.queryDeps)
	h.reads = NewReadReceiptService(h.readDeps)
}

func newHarness() *harness {
	rooms := testRooms()
	members := map[int64][]int64{
		communityRoomID: {10, 11},
		dmRoomID:        {10, 11},
	}
	h := &harness{
		members:       &fakeMembers{byRoom: members},
		readStatus:    &fakeReadStatus{status: &roomusecase.RoomReadStatus{UnreadCount: 3}},
		courseRead:    &fakeReadStatus{status: &roomusecase.RoomReadStatus{UnreadCount: 7}},
		markRead:      &fakeMarkRead{},
		markCourse:    &fakeMarkRead{},
		sendMessage:   &fakeSendMessage{},
		updateMessage: &fakeUpdateMessage{},
		deleteMessage: &fakeDeleteMessage{},
		checkWritable: &fakeCheckRoomWritable{},
		anonIdentity:  &fakeAnonIdentity{},
		getMessage:    &fakeGetMessage{},
		listMentions:  &fakeListMentions{},
		listMedia:     &fakeListMessageMedia{},
		listMessages:  &fakeListMessages{},
		listAround:    &fakeListMessagesAround{},
		events:        &fakeEvents{},
	}
	getRoom := &fakeGetRoom{rooms: rooms}
	h.accessDeps = AccessPolicyDeps{
		GetRoom:            getRoom,
		GetRoomMemberIDs:   h.members,
		CheckRoomWritable:  h.checkWritable,
		CheckBlockRelation: &fakeCheckBlockRelation{},
	}
	h.commandDeps = MessageCommandDeps{
		GetRoom:         getRoom,
		GetRoomUserRole: &fakeRoomUserRole{role: model.RoomUserRoleMember},
		GetMessage:      h.getMessage,
		// 保存処理は internal パッケージの口なので、テストからは束の非公開
		// フィールドへ直接 fake を差す（束の意味は usecase/chat/writers.go 参照）。
		Writers: MessageWriters{
			send:   h.sendMessage,
			update: h.updateMessage,
			delete: h.deleteMessage,
		},
		ResolveMentions:              &fakeResolveMentions{rooms: rooms},
		ListMentions:                 h.listMentions,
		ListMessageMedia:             h.listMedia,
		GetOrCreateAnonymousIdentity: h.anonIdentity,
		Events:                       h.events,
	}
	h.queryDeps = MessageQueryDeps{
		ListMessages:       h.listMessages,
		ListMessagesAround: h.listAround,
	}
	h.readDeps = ReadReceiptDeps{
		GetRoom:                 getRoom,
		GetRoomMemberIDs:        h.members,
		MarkRoomAsRead:          h.markRead,
		MarkCourseRoomAsRead:    h.markCourse,
		GetRoomReadStatus:       h.readStatus,
		GetCourseRoomReadStatus: h.courseRead,
		Events:                  h.events,
	}
	h.rebuild()
	return h
}

// --- 閲覧権限 ---------------------------------------------------------------

func TestEnsureReadAccess_CourseRoomIsOpenToAnyUser(t *testing.T) {
	h := newHarness()

	// 授業内チャットは F-04 で全授業公開。room_users に居なくても読める。
	room, err := h.access.EnsureReadAccess(ctxAsUser(99), courseRoomID)
	if err != nil {
		t.Fatalf("course rooms must be readable by any authenticated user, got: %v", err)
	}
	if room.ID != courseRoomID {
		t.Errorf("room ID = %d, want %d", room.ID, courseRoomID)
	}
}

func TestEnsureReadAccess_NonMemberIsRejectedInCommunity(t *testing.T) {
	h := newHarness()

	if _, err := h.access.EnsureReadAccess(ctxAsUser(99), communityRoomID); err == nil {
		t.Fatal("expected a rejection for a non-member of a community room")
	}
}

func TestEnsureReadAccess_MemberIsAllowed(t *testing.T) {
	h := newHarness()

	if _, err := h.access.EnsureReadAccess(ctxAsUser(10), communityRoomID); err != nil {
		t.Fatalf("members must be able to read their room, got: %v", err)
	}
}

func TestEnsureReadAccess_AdminMayReadCommunityButNotDM(t *testing.T) {
	h := newHarness()

	if _, err := h.access.EnsureReadAccess(ctxAsAdmin(99), communityRoomID); err != nil {
		t.Fatalf("admins must be able to read a community room they aren't a member of, got: %v", err)
	}
	if _, err := h.access.EnsureReadAccess(ctxAsAdmin(99), dmRoomID); err == nil {
		t.Fatal("admins must NOT gain access to DMs they aren't part of")
	}
}

// --- 書き込み権限 -----------------------------------------------------------

func TestEnsureWriteAccess_CourseRoomDelegatesToWritablePolicy(t *testing.T) {
	h := newHarness()
	h.checkWritable.err = apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")

	_, err := h.access.EnsureWriteAccess(ctxAsUser(99), courseRoomID)
	if err == nil {
		t.Fatal("expected an archived course room to reject writes")
	}
	if !h.checkWritable.called {
		t.Error("course rooms must be checked against the semester/timetable policy")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Errorf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
}

func TestEnsureWriteAccess_NonMemberIsRejected(t *testing.T) {
	h := newHarness()

	if _, err := h.access.EnsureWriteAccess(ctxAsUser(99), communityRoomID); err == nil {
		t.Fatal("expected a non-member to be rejected when writing to a community room")
	}
}

// --- 一覧取得 -----------------------------------------------------------------
// 一覧取得は別サービスになったが、閲覧権限は送信・既読と同じ AccessPolicy を
// 共有している。判定を写経し直していないことをここで押さえる。

func TestListMessages_GoesThroughTheSameReadPolicy(t *testing.T) {
	h := newHarness()

	if _, err := h.queries.ListMessages(ctxAsUser(99), ListMessagesInput{RoomID: communityRoomID, Limit: 20}); err == nil {
		t.Fatal("expected a non-member to be rejected when listing a community room's messages")
	}
	if h.listMessages.calls != 0 {
		t.Errorf("messages were loaded %d times for a rejected caller, want 0", h.listMessages.calls)
	}

	if _, err := h.queries.ListMessages(ctxAsUser(10), ListMessagesInput{RoomID: communityRoomID, Limit: 20}); err != nil {
		t.Fatalf("members must be able to list their room's messages, got: %v", err)
	}
	if h.listMessages.calls != 1 {
		t.Errorf("messages were loaded %d times, want 1", h.listMessages.calls)
	}

	// AroundID を渡したときだけ前後取得の経路へ振り分ける。
	around := int64(5)
	if _, err := h.queries.ListMessages(ctxAsUser(10), ListMessagesInput{RoomID: communityRoomID, Limit: 20, AroundID: &around}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.listAround.calls != 1 || h.listMessages.calls != 1 {
		t.Errorf("around calls = %d, plain calls = %d, want 1 and 1", h.listAround.calls, h.listMessages.calls)
	}
}

// --- 送信 -------------------------------------------------------------------

func TestSendMessage_CourseRoomAssignsAnonymousIdentityAtPostTime(t *testing.T) {
	h := newHarness()

	if _, err := h.commands.SendMessage(ctxAsUser(10), SendMessageInput{RoomID: courseRoomID, Content: "hi"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.anonIdentity.calls != 1 {
		t.Fatalf("anonymous identity allocations = %d, want 1 (採番は投稿時に確定させる)", h.anonIdentity.calls)
	}
	if h.anonIdentity.roomID != courseRoomID {
		t.Errorf("allocated in room %d, want %d", h.anonIdentity.roomID, courseRoomID)
	}
}

func TestSendMessage_NonCourseRoomDoesNotAllocateAnonymousIdentity(t *testing.T) {
	h := newHarness()

	if _, err := h.commands.SendMessage(ctxAsUser(10), SendMessageInput{RoomID: communityRoomID, Content: "hi"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.anonIdentity.calls != 0 {
		t.Errorf("anonymous identity allocations = %d, want 0 outside course rooms", h.anonIdentity.calls)
	}
}

func TestSendMessage_MentionsAreDroppedInCourseRoom(t *testing.T) {
	h := newHarness()

	// 授業内チャットは匿名なので実名メンションが成立してはいけない。クライアントが
	// mentionUserIDs を送ってきても、保存に渡るのは検証済み（＝空）の結果だけ。
	if _, err := h.commands.SendMessage(ctxAsUser(10), SendMessageInput{
		RoomID:         courseRoomID,
		Content:        "@someone hi",
		MentionUserIDs: []int64{11, 12},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.sendMessage.savedMentions) != 0 {
		t.Fatalf("saved mentions = %d, want 0 in a course room", len(h.sendMessage.savedMentions))
	}
}

func TestSendMessage_MentionsSurviveInCommunityRoom(t *testing.T) {
	h := newHarness()

	if _, err := h.commands.SendMessage(ctxAsUser(10), SendMessageInput{
		RoomID:         communityRoomID,
		Content:        "@someone hi",
		MentionUserIDs: []int64{11},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.sendMessage.savedMentions) != 1 {
		t.Fatalf("saved mentions = %d, want 1 in a community room", len(h.sendMessage.savedMentions))
	}
}

// --- roomID の照合 ----------------------------------------------------------

func TestDeleteMessage_RejectsMessageFromAnotherRoom(t *testing.T) {
	h := newHarness()
	// メッセージはコミュニティにあるのに、呼び出しは授業ルームを名乗っている。
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10}

	_, err := h.commands.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
	if err == nil {
		t.Fatal("expected a roomID/message mismatch to be rejected")
	}
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Errorf("error code = %s, want %s (存在の有無を漏らさないため not found に寄せる)", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if h.deleteMessage.called {
		t.Error("the message must not be deleted when the roomID does not match")
	}
}

func TestUpdateMessage_RejectsMessageFromAnotherRoom(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10}

	_, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: dmRoomID, MessageID: 5, Content: "x"})
	if err == nil {
		t.Fatal("expected a roomID/message mismatch to be rejected")
	}
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Errorf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if h.updateMessage.called {
		t.Error("the message must not be updated when the roomID does not match")
	}
}

// --- 授業ルームの削除ルール -------------------------------------------------

func TestDeleteMessage_ArchivedCourseRoomRejectsOwnDeletion(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: courseRoomID, UserID: 10}
	h.checkWritable.err = apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")

	_, err := h.commands.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
	if err == nil {
		t.Fatal("expected deletion in an archived course room to be rejected (質問・投票に揃えた)")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Errorf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
	if h.deleteMessage.called {
		t.Error("the message must not be deleted when the course room is no longer writable")
	}
}

func TestDeleteMessage_AdminMayDeleteInArchivedCourseRoom(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: courseRoomID, UserID: 10}
	h.checkWritable.err = apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")

	deleted, err := h.commands.DeleteMessage(ctxAsAdmin(99), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
	if err != nil {
		t.Fatalf("admins must keep being able to delete, got: %v", err)
	}
	if !deleted {
		t.Error("expected the message to be deleted")
	}
}

func TestDeleteMessage_WritableCourseRoomAllowsOwnDeletion(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: courseRoomID, UserID: 10}

	deleted, err := h.commands.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected the message to be deleted")
	}
}

func TestDeleteMessage_CommunityOwnerMayDeleteOthersMessages(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	h.commandDeps.GetRoomUserRole = &fakeRoomUserRole{role: model.RoomUserRoleOwner}
	h.rebuild()

	deleted, err := h.commands.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: communityRoomID, MessageID: 5})
	if err != nil {
		t.Fatalf("community owners must keep being able to delete others' messages, got: %v", err)
	}
	if !deleted {
		t.Error("expected the message to be deleted")
	}
}

func TestDeleteMessage_NonOwnerCannotDeleteOthersMessages(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}

	if _, err := h.commands.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: communityRoomID, MessageID: 5}); err == nil {
		t.Fatal("expected a plain member to be unable to delete someone else's message")
	}
}

// --- メンションが成立するルーム ---------------------------------------------

func TestMentionsSupported_OnlyCommunity(t *testing.T) {
	cases := map[string]struct {
		room *model.Room
		want bool
	}{
		"community": {&model.Room{Type: model.RoomTypeCommunity}, true},
		"course":    {&model.Room{Type: model.RoomTypeCourse}, false},
		"dm":        {&model.Room{Type: model.RoomTypeDM}, false},
		"nil":       {nil, false},
	}
	for name, c := range cases {
		if got := messageusecase.MentionsSupported(c.room); got != c.want {
			t.Errorf("MentionsSupported(%s) = %v, want %v", name, got, c.want)
		}
	}
}

// --- 退出・キック後の編集／削除 ---------------------------------------------
// 所有者かどうかしか見ていなかったため、コミュニティを退出（またはキック）された
// 利用者でも既知の message ID から自分の投稿を編集・削除できていた。

func TestUpdateMessage_NonMemberCannotEditOwnMessage(t *testing.T) {
	h := newHarness()
	// 12 は community のメンバー一覧に居ない＝退出済み。メッセージは本人のもの。
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 12}

	_, err := h.commands.UpdateMessage(ctxAsUser(12), UpdateMessageInput{RoomID: communityRoomID, MessageID: 5, Content: "x"})
	if err == nil {
		t.Fatal("expected a user who left the community to be unable to edit their own message")
	}
	if h.updateMessage.called {
		t.Error("the message must not be updated by a non-member")
	}
}

func TestDeleteMessage_NonMemberCannotDeleteOwnMessage(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 12}

	if _, err := h.commands.DeleteMessage(ctxAsUser(12), DeleteMessageInput{RoomID: communityRoomID, MessageID: 5}); err == nil {
		t.Fatal("expected a user who left the community to be unable to delete their own message")
	}
	if h.deleteMessage.called {
		t.Error("the message must not be deleted by a non-member")
	}
}

func TestUpdateMessage_MemberCanStillEditOwnMessage(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10}

	if _, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: communityRoomID, MessageID: 5, Content: "x"}); err != nil {
		t.Fatalf("members must keep being able to edit their own messages, got: %v", err)
	}
	if !h.updateMessage.called {
		t.Error("expected the message to be updated")
	}
}

// 管理者の扱いは EnsureReadAccess と揃える。閲覧できない非参加の DM を編集・削除
// できてしまうと方針が食い違うので、DM だけは管理者でも membership を要求する。
func TestUpdateMessage_AdminCannotEditInDMTheyAreNotPartOf(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: dmRoomID, UserID: 10}

	if _, err := h.commands.UpdateMessage(ctxAsAdmin(99), UpdateMessageInput{RoomID: dmRoomID, MessageID: 5, Content: "x"}); err == nil {
		t.Fatal("admins must NOT be able to edit messages in a DM they aren't part of (EnsureReadAccess と同じ規則)")
	}
	if h.updateMessage.called {
		t.Error("the message must not be updated by an admin outside the DM")
	}
}

func TestDeleteMessage_AdminCannotDeleteInDMTheyAreNotPartOf(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: dmRoomID, UserID: 10}

	if _, err := h.commands.DeleteMessage(ctxAsAdmin(99), DeleteMessageInput{RoomID: dmRoomID, MessageID: 5}); err == nil {
		t.Fatal("admins must NOT be able to delete messages in a DM they aren't part of")
	}
	if h.deleteMessage.called {
		t.Error("the message must not be deleted by an admin outside the DM")
	}
}

func TestDeleteMessage_AdminMayDeleteInCommunityTheyAreNotMemberOf(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10}

	deleted, err := h.commands.DeleteMessage(ctxAsAdmin(99), DeleteMessageInput{RoomID: communityRoomID, MessageID: 5})
	if err != nil {
		t.Fatalf("admins must keep being able to moderate community rooms, got: %v", err)
	}
	if !deleted {
		t.Error("expected the message to be deleted")
	}
}

func TestUpdateMessage_ArchivedCourseRoomRejectsEdit(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: courseRoomID, UserID: 10}
	h.checkWritable.err = apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")

	_, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: courseRoomID, MessageID: 5, Content: "x"})
	if err == nil {
		t.Fatal("expected an edit in an archived course room to be rejected")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Errorf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
	if h.updateMessage.called {
		t.Error("the message must not be updated when the course room is no longer writable")
	}
}

// --- 本文も添付も無いメッセージを作らせない ---------------------------------

func TestUpdateMessage_RejectsEmptyingAMessageWithoutMedia(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10, Content: "before"}

	_, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: communityRoomID, MessageID: 5, Content: "   "})
	if err == nil {
		t.Fatal("expected an edit that empties a message with no attachment to be rejected")
	}
	// 文言・エラーコードは生成時 (model.NewMessage) と同じ扱いにする。
	if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Errorf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeInvalidInput)
	}
	if err.Error() != "content or media is required" {
		t.Errorf("error = %q, want the same wording as model.NewMessage", err.Error())
	}
	if h.updateMessage.called {
		t.Error("the message must not be saved with neither content nor media")
	}
}

func TestUpdateMessage_AllowsEmptyContentWhenMediaRemains(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10, Content: "before"}
	h.listMedia.byMessage = map[int64][]*model.Media{5: {{ID: 1}}}

	if _, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: communityRoomID, MessageID: 5, Content: ""}); err != nil {
		t.Fatalf("a message that still has an attachment may have an empty body, got: %v", err)
	}
	if !h.updateMessage.called {
		t.Error("expected the message to be updated")
	}
}

func TestUpdateMessage_RejectsEmptyingWhenAttachmentsCannotBeChecked(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10, Content: "before"}
	h.listMedia.err = errors.New("db is down")

	// 添付の有無が分からないまま空本文を通すと不変条件を破りうるので、失敗させる。
	if _, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: communityRoomID, MessageID: 5, Content: ""}); err == nil {
		t.Fatal("expected the edit to fail when the attachment count cannot be read")
	}
	if h.updateMessage.called {
		t.Error("the message must not be saved while the attachment state is unknown")
	}
}

// --- 編集前メンションが引けないときの再通知抑止 -----------------------------

func TestUpdateMessage_SuppressesMentionNotificationsWhenTheDiffIsUnknown(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10}
	h.listMentions.err = errors.New("db is down")

	if _, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{
		RoomID:         communityRoomID,
		MessageID:      5,
		Content:        "@someone hi",
		MentionUserIDs: []int64{11},
	}); err != nil {
		t.Fatalf("the edit itself must still be saved, got: %v", err)
	}
	if !h.updateMessage.called {
		t.Error("expected the edit to be saved even though the mention diff is unknown")
	}
	if len(h.events.updated) != 1 {
		t.Fatalf("MessageUpdated events = %d, want 1", len(h.events.updated))
	}
	if got := len(h.events.updated[0].AddedMentions); got != 0 {
		t.Errorf("added mentions = %d, want 0 (差分が不明なときは再通知しない)", got)
	}
}

func TestUpdateMessage_NotifiesOnlyNewlyAddedMentions(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 10}
	h.listMentions.byMessage = map[int64][]*model.Mention{5: {{UserID: 11}}}

	if _, err := h.commands.UpdateMessage(ctxAsUser(10), UpdateMessageInput{
		RoomID:         communityRoomID,
		MessageID:      5,
		Content:        "@a @b hi",
		MentionUserIDs: []int64{11, 12},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h.events.updated) != 1 {
		t.Fatalf("MessageUpdated events = %d, want 1", len(h.events.updated))
	}
	added := h.events.updated[0].AddedMentions
	if len(added) != 1 || added[0].UserID != 12 {
		t.Fatalf("added mentions = %+v, want only user 12 (11 は編集前から居るので再通知しない)", added)
	}
}

// --- 既読 -------------------------------------------------------------------

// GetReadStatus は単体で閲覧権限を確かめる。リゾルバが先に EnsureReadAccess を
// 通しているから安全、では公開メソッドの契約として不完全なため。
func TestGetReadStatus_NonMemberIsRejected(t *testing.T) {
	h := newHarness()

	if _, err := h.reads.GetReadStatus(ctxAsUser(99), communityRoomID); err == nil {
		t.Fatal("expected a non-member to be rejected when reading a community room's read status")
	}
	if _, err := h.reads.GetReadStatus(ctxAsAdmin(99), dmRoomID); err == nil {
		t.Fatal("expected an admin who is not part of the DM to be rejected")
	}
	if h.readStatus.calls != 0 {
		t.Fatalf("read status was loaded %d times for rejected callers, want 0", h.readStatus.calls)
	}
}

func TestGetReadStatus_MemberIsAllowed(t *testing.T) {
	h := newHarness()

	status, err := h.reads.GetReadStatus(ctxAsUser(10), communityRoomID)
	if err != nil {
		t.Fatalf("members must be able to read their own read status, got: %v", err)
	}
	if status.UnreadCount != 3 || h.readStatus.roomID != communityRoomID {
		t.Fatalf("status = %+v (room %d), want the room_users 経路", status, h.readStatus.roomID)
	}
}

// 授業内チャットは F-04 で全授業公開なので、履修していなくても未読は引ける。
// 既読位置の置き場は course_room_reads 側（room_users ではない）。
func TestGetReadStatus_CourseRoomIsOpenAndUsesCourseStore(t *testing.T) {
	h := newHarness()

	status, err := h.reads.GetReadStatus(ctxAsUser(99), courseRoomID)
	if err != nil {
		t.Fatalf("course rooms must be readable by any authenticated user, got: %v", err)
	}
	if status.UnreadCount != 7 || h.courseRead.calls != 1 {
		t.Fatalf("status = %+v (course store calls = %d), want the course_room_reads 経路", status, h.courseRead.calls)
	}
	if h.readStatus.calls != 0 {
		t.Error("course rooms must not go through the room_users read store")
	}
}

// 既に EnsureReadAccess を通した呼び出し元（Room リゾルバ）は、権限判定を
// もう一度走らせない口を使う。ルーム表示のたびに membership を2回引かないため。
func TestReadStatusOfAuthorizedRoom_DoesNotRecheckMembership(t *testing.T) {
	h := newHarness()

	room, err := h.access.EnsureReadAccess(ctxAsUser(10), communityRoomID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before := h.members.calls

	if _, err := h.reads.ReadStatusOfAuthorizedRoom(ctxAsUser(10), room); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.members.calls != before {
		t.Fatalf("membership lookups = %d, want no additional lookup after EnsureReadAccess (was %d)", h.members.calls, before)
	}
	if h.readStatus.calls != 1 {
		t.Fatalf("read status loads = %d, want 1", h.readStatus.calls)
	}
}

// 既読は「読めるか」ではなく「自分の既読位置を持てるか」なので membership が要る。
// 管理者は非メンバーのコミュニティを閲覧できるが room_users の行が無く、既読位置を
// 書く先が無いため通さない（read.go のコメント参照）。
func TestMarkAsRead_RequiresMembershipEvenForAdmins(t *testing.T) {
	h := newHarness()

	if err := h.reads.MarkAsRead(ctxAsAdmin(99), communityRoomID); err == nil {
		t.Fatal("expected an admin without a room_users row to be rejected")
	}
	if h.markRead.calls != 0 {
		t.Fatalf("read position was written %d times for a non-member, want 0", h.markRead.calls)
	}

	if err := h.reads.MarkAsRead(ctxAsUser(10), communityRoomID); err != nil {
		t.Fatalf("members must be able to mark a room as read, got: %v", err)
	}
	if h.markRead.calls != 1 || h.markRead.roomID != communityRoomID {
		t.Fatalf("markRoomAsRead calls = %d (room %d), want 1 for room %d", h.markRead.calls, h.markRead.roomID, communityRoomID)
	}
}

// 授業ルームは room_users を使わないので、既読位置は course_room_reads 側へ。
func TestMarkAsRead_CourseRoomUsesCourseStore(t *testing.T) {
	h := newHarness()

	if err := h.reads.MarkAsRead(ctxAsUser(99), courseRoomID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.markCourse.calls != 1 || h.markRead.calls != 0 {
		t.Fatalf("course store calls = %d, room_users calls = %d, want 1 and 0", h.markCourse.calls, h.markRead.calls)
	}
}
