package graph

// テスト項番18-31, 37-41 に対応する、GraphQL resolver 層(認可判定の実体)の
// テスト。SendMessage/DeleteMessage/UpdateMessage/Messages はいずれも
// mutationResolver{*Resolver} / queryResolver{*Resolver} のメソッドとして
// 呼び出せるため、Resolver に各 usecase のフェイクを注入して直接呼び出す。
//
// 項番31(「他組織のlessonIdを閲覧」)について: このアプリのデータモデルには
// 組織(テナント)の概念が存在せず、管理者は全授業のチャットを閲覧できる設計に
// なっている。そのため項番31は「別コースのlessonIdを指定した場合にそのコースの
// 履歴だけが返る」こと(項番27のテストで検証)として読み替えている。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/model"
	blockusecase "github.com/Cityboypenguin/SPACE-server/usecase/block"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	"github.com/rs/zerolog"
)

// ---- fakes -----------------------------------------------------------

type fakeGetRoomUseCase struct {
	room *model.Room
	err  error
}

func (f fakeGetRoomUseCase) Execute(_ context.Context, _ int64) (*model.Room, error) {
	return f.room, f.err
}

type fakeGetUserIDsByRoomIDUseCase struct {
	ids []int64
	err error
}

func (f fakeGetUserIDsByRoomIDUseCase) Execute(_ context.Context, _ int64) ([]int64, error) {
	return f.ids, f.err
}

type fakeCheckRoomWritableUseCase struct {
	err error
}

func (f fakeCheckRoomWritableUseCase) Execute(_ context.Context, _ int64) error {
	return f.err
}

var _ courseusecase.CheckRoomWritableUseCase = fakeCheckRoomWritableUseCase{}

type fakeCheckBlockRelationUseCase struct {
	blocked bool
}

func (f fakeCheckBlockRelationUseCase) Execute(_ context.Context, _, _ int64) (bool, error) {
	return f.blocked, nil
}

var _ blockusecase.CheckBlockRelationUseCase = fakeCheckBlockRelationUseCase{}

// recordingSendMessageUseCase records every call so tests can assert whether
// SendMessage was (or, more importantly, was NOT) reached after an authz check.
type recordingSendMessageUseCase struct {
	calls  []struct{ roomID, userID int64 }
	result *model.Message
	err    error
}

func (f *recordingSendMessageUseCase) Execute(_ context.Context, roomID, userID int64, _ string, _ []messageusecase.MediaInput) (*model.Message, error) {
	f.calls = append(f.calls, struct{ roomID, userID int64 }{roomID, userID})
	return f.result, f.err
}

type erroringGetMembersUnreadCountsUseCase struct{}

func (erroringGetMembersUnreadCountsUseCase) Execute(_ context.Context, _ int64, _ int64) (map[int64]int, error) {
	// SendMessage only touches SSEBroker when this succeeds; returning an error
	// lets tests avoid wiring a real *sse.Broker.
	return nil, errors.New("unread counts unavailable in test")
}

var _ roomusecase.GetMembersUnreadCountsUseCase = erroringGetMembersUnreadCountsUseCase{}

type fakeGetMessageByIDUseCase struct {
	msg *model.Message
	err error
}

func (f fakeGetMessageByIDUseCase) Execute(_ context.Context, _ int64) (*model.Message, error) {
	return f.msg, f.err
}

type recordingDeleteMessageUseCase struct {
	calls  []struct{ messageID, deletedBy int64 }
	result bool
	err    error
}

func (f *recordingDeleteMessageUseCase) Execute(_ context.Context, messageID int64, deletedBy int64) (bool, error) {
	f.calls = append(f.calls, struct{ messageID, deletedBy int64 }{messageID, deletedBy})
	return f.result, f.err
}

type fakeGetRoomUserRoleUseCase struct {
	role string
	err  error
}

func (f fakeGetRoomUserRoleUseCase) Execute(_ context.Context, _, _ int64) (string, error) {
	return f.role, f.err
}

type recordingUpdateMessageUseCase struct {
	calls  int
	result *model.Message
	err    error
}

func (f *recordingUpdateMessageUseCase) Execute(_ context.Context, _ int64, _ model.UpdateMessageParam) (*model.Message, error) {
	f.calls++
	return f.result, f.err
}

type fakeListMessagesUseCase struct {
	messages []*model.Message
	err      error
}

func (f fakeListMessagesUseCase) Execute(_ context.Context, _ int64, _ int, _ *int64, _ *int64, _ *time.Time) ([]*model.Message, bool, bool, error) {
	return f.messages, false, false, f.err
}

var _ messageusecase.ListMessagesUseCase = fakeListMessagesUseCase{}

// ---- context / id helpers ---------------------------------------------

func ctxAsUser(id int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: id, Role: "user"})
}

func ctxAsAdmin(id int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: id, Role: "admin"})
}

func roomGID(id int64) string    { return opaqueid.Encode("room", id) }
func messageGID(id int64) string { return opaqueid.Encode("message", id) }

// ---- SendMessage: 項番18-21,24 -----------------------------------------

func TestSendMessage_CourseRoom_RegisteredStudent_Succeeds(t *testing.T) {
	send := &recordingSendMessageUseCase{result: &model.Message{ID: 1, RoomID: 10, UserID: 42, Content: "hi"}}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:                roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 10, Type: model.RoomTypeCourse}}),
			SendMessageUseCase:            send,
			GetMembersUnreadCountsUseCase: erroringGetMembersUnreadCountsUseCase{},
		},
		CourseUseCases: CourseUseCases{CheckRoomWritableUseCase: fakeCheckRoomWritableUseCase{err: nil}},
		PubSub:         pubsub.New(),
	}}

	msg, err := r.SendMessage(ctxAsUser(42), roomGID(10), "hi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected a message to be returned")
	}
	if len(send.calls) != 1 {
		t.Fatalf("expected SendMessageUseCase to be called once, got %d", len(send.calls))
	}
	// 項番24: userID はリクエストのどこにも現れず、認証済みクレームの ID が
	// そのまま使われる(改ざんの余地がないことの構造的確認)。
	if send.calls[0].userID != 42 {
		t.Errorf("SendMessageUseCase called with userID %d, want authenticated claims.ID 42", send.calls[0].userID)
	}
}

func TestSendMessage_CourseRoom_UnregisteredStudent_Forbidden(t *testing.T) {
	send := &recordingSendMessageUseCase{result: &model.Message{ID: 1}}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:     roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 10, Type: model.RoomTypeCourse}}),
			SendMessageUseCase: send,
		},
		CourseUseCases: CourseUseCases{CheckRoomWritableUseCase: fakeCheckRoomWritableUseCase{err: errors.New("forbidden: not registered")}},
		PubSub:         pubsub.New(),
	}}

	if _, err := r.SendMessage(ctxAsUser(42), roomGID(10), "hi", nil); err == nil {
		t.Fatal("expected error for unregistered student posting to a course room")
	}
	if len(send.calls) != 0 {
		t.Fatal("SendMessageUseCase must not be called when writability check fails")
	}
}

func TestSendMessage_NonMemberOfNonCourseRoom_Forbidden(t *testing.T) {
	// 項番20,25: 参加していないルーム(別授業=別ルームに読み替え)への roomID を
	// 指定しても、membership チェックで拒否される。
	send := &recordingSendMessageUseCase{result: &model.Message{ID: 1}}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:            roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 20, Type: model.RoomTypeCommunity}}),
			GetUserIDsByRoomIDUseCase: roomusecase.GetUserIDsByRoomIDUseCase(fakeGetUserIDsByRoomIDUseCase{ids: []int64{1, 2, 3}}),
			SendMessageUseCase:        send,
		},
		PubSub: pubsub.New(),
	}}

	_, err := r.SendMessage(ctxAsUser(42), roomGID(20), "hi", nil)
	if err == nil {
		t.Fatal("expected error posting to a room the caller is not a member of")
	}
	if !strings.Contains(err.Error(), "not a member") {
		t.Errorf("error = %q, want it to mention membership", err.Error())
	}
	if len(send.calls) != 0 {
		t.Fatal("SendMessageUseCase must not be called for a non-member")
	}
}

func TestSendMessage_Unauthenticated_Forbidden(t *testing.T) {
	send := &recordingSendMessageUseCase{result: &model.Message{ID: 1}}
	r := &mutationResolver{&Resolver{MessageRoomUseCases: MessageRoomUseCases{SendMessageUseCase: send}}}

	if _, err := r.SendMessage(context.Background(), roomGID(10), "hi", nil); err == nil {
		t.Fatal("expected error for unauthenticated send")
	}
	if len(send.calls) != 0 {
		t.Fatal("SendMessageUseCase must not be called without authentication")
	}
}

// ---- DeleteMessage: 項番37-41 -------------------------------------------

func TestDeleteMessage_Admin_Succeeds_AndAudited(t *testing.T) {
	del := &recordingDeleteMessageUseCase{result: true}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase: messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: &model.Message{ID: 5, RoomID: 10, UserID: 1}}),
			DeleteMessageUseCase:  del,
		},
		PubSub: pubsub.New(),
	}}

	var buf strings.Builder
	restore := swapLogger(&buf)
	defer restore()

	const adminID = int64(99)
	deleted, err := r.DeleteMessage(ctxAsAdmin(adminID), roomGID(10), messageGID(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Fatal("expected deleted = true")
	}
	if len(del.calls) != 1 || del.calls[0].deletedBy != adminID {
		t.Fatalf("DeleteMessageUseCase should be called once with deletedBy=%d, got %+v", adminID, del.calls)
	}

	// 項番41: 削除者ID・対象messageIdが監査ログに記録される。
	logOutput := buf.String()
	if !strings.Contains(logOutput, "message_deleted") {
		t.Errorf("expected audit log to contain event=message_deleted, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"actor_id":99`) {
		t.Errorf("expected audit log to record actor_id=99, got: %s", logOutput)
	}
}

func TestDeleteMessage_NonOwnerStudent_Forbidden(t *testing.T) {
	// 項番38相当: 自分の投稿ではないメッセージを一般生徒(非管理者)が削除しようとすると拒否される。
	del := &recordingDeleteMessageUseCase{result: true}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase: messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: &model.Message{ID: 5, RoomID: 10, UserID: 1}}),
			GetRoomUseCase:        roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 10, Type: model.RoomTypeCourse}}),
			DeleteMessageUseCase:  del,
		},
		PubSub: pubsub.New(),
	}}

	_, err := r.DeleteMessage(ctxAsUser(42), roomGID(10), messageGID(5))
	if err == nil {
		t.Fatal("expected forbidden error deleting another user's message")
	}
	if len(del.calls) != 0 {
		t.Fatal("DeleteMessageUseCase must not be called when forbidden")
	}
}

func TestDeleteMessage_NonexistentID_Error(t *testing.T) {
	del := &recordingDeleteMessageUseCase{result: true}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase: messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: nil}),
			DeleteMessageUseCase:  del,
		},
		PubSub: pubsub.New(),
	}}

	if _, err := r.DeleteMessage(ctxAsAdmin(99), roomGID(10), messageGID(999)); err == nil {
		t.Fatal("expected error deleting a nonexistent message")
	}
	if len(del.calls) != 0 {
		t.Fatal("DeleteMessageUseCase must not be called for a nonexistent message")
	}
}

func TestDeleteMessage_AlreadyDeleted_NoInconsistency(t *testing.T) {
	// 項番40: GetMessageByIDUseCase は削除済みメッセージを nil として返す想定
	// (usecase 層のフィルタ挙動)。2回目の削除操作は「見つからない」と同じ経路を
	// たどり、DeleteMessageUseCase が再度呼ばれてデータが壊れることはない。
	del := &recordingDeleteMessageUseCase{result: true}
	getMsg := fakeGetMessageByIDUseCase{msg: nil} // 既に削除済み = 一般取得では見えない
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase: messageusecase.GetMessageByIDUseCase(getMsg),
			DeleteMessageUseCase:  del,
		},
		PubSub: pubsub.New(),
	}}

	if _, err := r.DeleteMessage(ctxAsAdmin(99), roomGID(10), messageGID(5)); err == nil {
		t.Fatal("expected error re-deleting an already-deleted message")
	}
	if len(del.calls) != 0 {
		t.Fatal("DeleteMessageUseCase must not be invoked again for an already-deleted message")
	}
}

// ---- UpdateMessage: 所有者チェックの不具合修正 -----------------------------

func TestUpdateMessage_NonOwnerNonAdmin_Forbidden(t *testing.T) {
	upd := &recordingUpdateMessageUseCase{result: &model.Message{ID: 5}}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase: messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: &model.Message{ID: 5, RoomID: 10, UserID: 1}}),
			UpdateMessageUseCase:  upd,
		},
		PubSub: pubsub.New(),
	}}

	_, err := r.UpdateMessage(ctxAsUser(42), roomGID(10), messageGID(5), "書き換え")
	if err == nil {
		t.Fatal("expected forbidden error updating another user's message")
	}
	if upd.calls != 0 {
		t.Fatal("UpdateMessageUseCase must not be called for a non-owner, non-admin caller")
	}
}

func TestUpdateMessage_Owner_Succeeds(t *testing.T) {
	upd := &recordingUpdateMessageUseCase{result: &model.Message{ID: 5, UserID: 42, Content: "書き換え"}}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase: messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: &model.Message{ID: 5, RoomID: 10, UserID: 42}}),
			UpdateMessageUseCase:  upd,
		},
		PubSub: pubsub.New(),
	}}

	if _, err := r.UpdateMessage(ctxAsUser(42), roomGID(10), messageGID(5), "書き換え"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upd.calls != 1 {
		t.Fatalf("expected UpdateMessageUseCase to be called once, got %d", upd.calls)
	}
}

func TestUpdateMessage_Admin_CanEditOthers(t *testing.T) {
	upd := &recordingUpdateMessageUseCase{result: &model.Message{ID: 5, UserID: 1, Content: "修正"}}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase: messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: &model.Message{ID: 5, RoomID: 10, UserID: 1}}),
			UpdateMessageUseCase:  upd,
		},
		PubSub: pubsub.New(),
	}}

	if _, err := r.UpdateMessage(ctxAsAdmin(99), roomGID(10), messageGID(5), "修正"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upd.calls != 1 {
		t.Fatalf("expected UpdateMessageUseCase to be called once, got %d", upd.calls)
	}
}

func TestDeleteMessage_CommunityOwnerCanDeleteMembersMessage(t *testing.T) {
	del := &recordingDeleteMessageUseCase{result: true}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase:  messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: &model.Message{ID: 5, RoomID: 10, UserID: 1}}),
			GetRoomUseCase:         roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 10, Type: model.RoomTypeCommunity}}),
			GetRoomUserRoleUseCase: roomusecase.GetRoomUserRoleUseCase(fakeGetRoomUserRoleUseCase{role: model.RoomUserRoleOwner}),
			DeleteMessageUseCase:   del,
		},
		PubSub: pubsub.New(),
	}}

	deleted, err := r.DeleteMessage(ctxAsUser(42), roomGID(10), messageGID(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted || len(del.calls) != 1 {
		t.Fatalf("community owner should be able to delete a member's message: deleted=%v calls=%d", deleted, len(del.calls))
	}
}

func TestDeleteMessage_CommunityNonOwnerMember_Forbidden(t *testing.T) {
	del := &recordingDeleteMessageUseCase{result: true}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetMessageByIDUseCase:  messageusecase.GetMessageByIDUseCase(fakeGetMessageByIDUseCase{msg: &model.Message{ID: 5, RoomID: 10, UserID: 1}}),
			GetRoomUseCase:         roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 10, Type: model.RoomTypeCommunity}}),
			GetRoomUserRoleUseCase: roomusecase.GetRoomUserRoleUseCase(fakeGetRoomUserRoleUseCase{role: model.RoomUserRoleMember}),
			DeleteMessageUseCase:   del,
		},
		PubSub: pubsub.New(),
	}}

	if _, err := r.DeleteMessage(ctxAsUser(42), roomGID(10), messageGID(5)); err == nil {
		t.Fatal("a plain (non-owner) community member must not delete another member's message")
	}
	if len(del.calls) != 0 {
		t.Fatal("DeleteMessageUseCase must not be called")
	}
}

// DMでブロック関係にある相手には送信できない(SendMessage の権限判定の一部)。
func TestSendMessage_DMBlockedRecipient_Forbidden(t *testing.T) {
	send := &recordingSendMessageUseCase{result: &model.Message{ID: 1}}
	r := &mutationResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:            roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 30, Type: model.RoomTypeDM}}),
			GetUserIDsByRoomIDUseCase: roomusecase.GetUserIDsByRoomIDUseCase(fakeGetUserIDsByRoomIDUseCase{ids: []int64{42, 7}}),
			SendMessageUseCase:        send,
		},
		CheckBlockRelationUseCase: blockusecase.CheckBlockRelationUseCase(fakeCheckBlockRelationUseCase{blocked: true}),
		PubSub:                    pubsub.New(),
	}}

	if _, err := r.SendMessage(ctxAsUser(42), roomGID(30), "hi", nil); err == nil {
		t.Fatal("expected send to be rejected when the DM partner is blocked")
	}
	if len(send.calls) != 0 {
		t.Fatal("SendMessageUseCase must not be called when blocked")
	}
}

// ---- Messages query: 項番26-30 -------------------------------------------

func TestMessages_CourseRoom_AdminCanView(t *testing.T) {
	// 項番26: 管理者は授業チャット履歴を閲覧できる。授業チャットは全公開のため
	// membership チェックは行われない。
	want := []*model.Message{{ID: 1, RoomID: 10}, {ID: 2, RoomID: 10}}
	r := &queryResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:      roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 10, Type: model.RoomTypeCourse}}),
			ListMessagesUseCase: fakeListMessagesUseCase{messages: want},
		},
	}}

	page, err := r.Messages(ctxAsAdmin(99), roomGID(10), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(page.Items))
	}
}

func TestMessages_NonCourseRoom_NonMemberNonAdmin_Forbidden(t *testing.T) {
	r := &queryResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:            roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 20, Type: model.RoomTypeCommunity}}),
			GetUserIDsByRoomIDUseCase: roomusecase.GetUserIDsByRoomIDUseCase(fakeGetUserIDsByRoomIDUseCase{ids: []int64{1, 2}}),
			ListMessagesUseCase:       fakeListMessagesUseCase{messages: []*model.Message{{ID: 1, RoomID: 20}}},
		},
	}}

	if _, err := r.Messages(ctxAsUser(42), roomGID(20), nil, nil, nil, nil); err == nil {
		t.Fatal("expected forbidden error for a non-member, non-admin caller")
	}
}

// 項番29相当: 非管理者(一般生徒)が、メンバーでもない非公開ルームの履歴を
// 取得しようとしても拒否される(=管理者専用の閲覧経路は使えない)。
func TestMessages_NonCourseRoom_NonAdminStudent_Forbidden(t *testing.T) {
	r := &queryResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase:            roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 20, Type: model.RoomTypeCommunity}}),
			GetUserIDsByRoomIDUseCase: roomusecase.GetUserIDsByRoomIDUseCase(fakeGetUserIDsByRoomIDUseCase{ids: []int64{1, 2}}),
		},
	}}

	if _, err := r.Messages(ctxAsUser(999), roomGID(20), nil, nil, nil, nil); err == nil {
		t.Fatal("expected forbidden error for a non-admin, non-member student")
	}
}

// 項番28: 存在しない lessonId(roomId) を指定すると適切なエラーになる。
func TestMessages_NonexistentRoom_ReturnsError(t *testing.T) {
	r := &queryResolver{&Resolver{
		MessageRoomUseCases: MessageRoomUseCases{
			GetRoomUseCase: roomusecase.GetRoomUseCase(fakeGetRoomUseCase{err: errors.New("room not found: 999")}),
		},
	}}

	_, err := r.Messages(ctxAsAdmin(99), roomGID(999), nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for a nonexistent room id")
	}
	if !strings.Contains(err.Error(), "room not found") {
		t.Errorf("error = %q, want it to mention the room was not found", err.Error())
	}
}

// 項番30: 未ログインでの履歴取得は認証エラーになる。
func TestMessages_Unauthenticated_Error(t *testing.T) {
	r := &queryResolver{&Resolver{}}
	if _, err := r.Messages(context.Background(), roomGID(10), nil, nil, nil, nil); err == nil {
		t.Fatal("expected an authentication error")
	}
}

// 項番27: 別々のコースを指定すればそれぞれ別々の履歴だけが返る
// (ListMessagesUseCase に渡す roomID が指定どおりに使い分けられることの確認)。
func TestMessages_DifferentCourses_ReturnDistinctHistories(t *testing.T) {
	roomA := []*model.Message{{ID: 1, RoomID: 10}}
	roomB := []*model.Message{{ID: 2, RoomID: 20}}

	rA := &queryResolver{&Resolver{MessageRoomUseCases: MessageRoomUseCases{
		GetRoomUseCase:      roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 10, Type: model.RoomTypeCourse}}),
		ListMessagesUseCase: fakeListMessagesUseCase{messages: roomA},
	}}}
	rB := &queryResolver{&Resolver{MessageRoomUseCases: MessageRoomUseCases{
		GetRoomUseCase:      roomusecase.GetRoomUseCase(fakeGetRoomUseCase{room: &model.Room{ID: 20, Type: model.RoomTypeCourse}}),
		ListMessagesUseCase: fakeListMessagesUseCase{messages: roomB},
	}}}

	pageA, err := rA.Messages(ctxAsUser(1), roomGID(10), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for room A: %v", err)
	}
	pageB, err := rB.Messages(ctxAsUser(1), roomGID(20), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for room B: %v", err)
	}
	if len(pageA.Items) != 1 || pageA.Items[0].ID != encodeGraphID("message", 1) {
		t.Errorf("room A page should contain only message 1, got %+v", pageA.Items)
	}
	if len(pageB.Items) != 1 || pageB.Items[0].ID != encodeGraphID("message", 2) {
		t.Errorf("room B page should contain only message 2, got %+v", pageB.Items)
	}
}

// ---- swapLogger ---------------------------------------------------------

// swapLogger redirects the package-level audit logger to buf for the duration
// of a test and returns a func to restore it. Not safe to use with t.Parallel.
func swapLogger(buf *strings.Builder) (restore func()) {
	prev := logger.Log
	logger.Log = zerolog.New(buf)
	return func() { logger.Log = prev }
}
