package chat

import (
	"context"
	"fmt"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
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
}

func (f *fakeMembers) Execute(_ context.Context, roomID int64) ([]int64, error) {
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

type fakeListMentions struct{}

func (f *fakeListMentions) Execute(_ context.Context, _ []int64) (map[int64][]*model.Mention, error) {
	return nil, nil
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

type harness struct {
	deps          Deps
	service       Service
	sendMessage   *fakeSendMessage
	updateMessage *fakeUpdateMessage
	deleteMessage *fakeDeleteMessage
	checkWritable *fakeCheckRoomWritable
	anonIdentity  *fakeAnonIdentity
	getMessage    *fakeGetMessage
}

func newHarness() *harness {
	rooms := testRooms()
	members := map[int64][]int64{
		communityRoomID: {10, 11},
		dmRoomID:        {10, 11},
	}
	h := &harness{
		sendMessage:   &fakeSendMessage{},
		updateMessage: &fakeUpdateMessage{},
		deleteMessage: &fakeDeleteMessage{},
		checkWritable: &fakeCheckRoomWritable{},
		anonIdentity:  &fakeAnonIdentity{},
		getMessage:    &fakeGetMessage{},
	}
	h.deps = Deps{
		GetRoom:                      &fakeGetRoom{rooms: rooms},
		GetRoomMemberIDs:             &fakeMembers{byRoom: members},
		GetRoomUserRole:              &fakeRoomUserRole{role: model.RoomUserRoleMember},
		CheckRoomWritable:            h.checkWritable,
		CheckBlockRelation:           &fakeCheckBlockRelation{},
		GetMessage:                   h.getMessage,
		SendMessage:                  h.sendMessage,
		UpdateMessage:                h.updateMessage,
		DeleteMessage:                h.deleteMessage,
		ResolveMentions:              &fakeResolveMentions{rooms: rooms},
		ListMentions:                 &fakeListMentions{},
		GetOrCreateAnonymousIdentity: h.anonIdentity,
	}
	h.service = NewService(h.deps)
	return h
}

// --- 閲覧権限 ---------------------------------------------------------------

func TestEnsureReadAccess_CourseRoomIsOpenToAnyUser(t *testing.T) {
	h := newHarness()

	// 授業内チャットは F-04 で全授業公開。room_users に居なくても読める。
	room, err := h.service.EnsureReadAccess(ctxAsUser(99), courseRoomID)
	if err != nil {
		t.Fatalf("course rooms must be readable by any authenticated user, got: %v", err)
	}
	if room.ID != courseRoomID {
		t.Errorf("room ID = %d, want %d", room.ID, courseRoomID)
	}
}

func TestEnsureReadAccess_NonMemberIsRejectedInCommunity(t *testing.T) {
	h := newHarness()

	if _, err := h.service.EnsureReadAccess(ctxAsUser(99), communityRoomID); err == nil {
		t.Fatal("expected a rejection for a non-member of a community room")
	}
}

func TestEnsureReadAccess_MemberIsAllowed(t *testing.T) {
	h := newHarness()

	if _, err := h.service.EnsureReadAccess(ctxAsUser(10), communityRoomID); err != nil {
		t.Fatalf("members must be able to read their room, got: %v", err)
	}
}

func TestEnsureReadAccess_AdminMayReadCommunityButNotDM(t *testing.T) {
	h := newHarness()

	if _, err := h.service.EnsureReadAccess(ctxAsAdmin(99), communityRoomID); err != nil {
		t.Fatalf("admins must be able to read a community room they aren't a member of, got: %v", err)
	}
	if _, err := h.service.EnsureReadAccess(ctxAsAdmin(99), dmRoomID); err == nil {
		t.Fatal("admins must NOT gain access to DMs they aren't part of")
	}
}

// --- 書き込み権限 -----------------------------------------------------------

func TestEnsureWriteAccess_CourseRoomDelegatesToWritablePolicy(t *testing.T) {
	h := newHarness()
	h.checkWritable.err = apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")

	_, err := h.service.EnsureWriteAccess(ctxAsUser(99), courseRoomID)
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

	if _, err := h.service.EnsureWriteAccess(ctxAsUser(99), communityRoomID); err == nil {
		t.Fatal("expected a non-member to be rejected when writing to a community room")
	}
}

// --- 送信 -------------------------------------------------------------------

func TestSendMessage_CourseRoomAssignsAnonymousIdentityAtPostTime(t *testing.T) {
	h := newHarness()

	if _, err := h.service.SendMessage(ctxAsUser(10), SendMessageInput{RoomID: courseRoomID, Content: "hi"}); err != nil {
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

	if _, err := h.service.SendMessage(ctxAsUser(10), SendMessageInput{RoomID: communityRoomID, Content: "hi"}); err != nil {
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
	if _, err := h.service.SendMessage(ctxAsUser(10), SendMessageInput{
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

	if _, err := h.service.SendMessage(ctxAsUser(10), SendMessageInput{
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

	_, err := h.service.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
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

	_, err := h.service.UpdateMessage(ctxAsUser(10), UpdateMessageInput{RoomID: dmRoomID, MessageID: 5, Content: "x"})
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

	_, err := h.service.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
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

	deleted, err := h.service.DeleteMessage(ctxAsAdmin(99), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
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

	deleted, err := h.service.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: courseRoomID, MessageID: 5})
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
	h.deps.GetRoomUserRole = &fakeRoomUserRole{role: model.RoomUserRoleOwner}
	h.service = NewService(h.deps)

	deleted, err := h.service.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: communityRoomID, MessageID: 5})
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

	if _, err := h.service.DeleteMessage(ctxAsUser(10), DeleteMessageInput{RoomID: communityRoomID, MessageID: 5}); err == nil {
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
