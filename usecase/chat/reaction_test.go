package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeReactions はリアクションの保存先の代役。付けた・外した記録だけを持つ。
type fakeReactions struct {
	repository.MessageReactionRepository
	added   []fakeReactionCall
	removed []fakeReactionCall
}

type fakeReactionCall struct {
	messageID, userID int64
	emoji             string
}

func (f *fakeReactions) AddMessageReaction(_ context.Context, messageID, userID int64, emoji string) error {
	f.added = append(f.added, fakeReactionCall{messageID, userID, emoji})
	return nil
}

func (f *fakeReactions) RemoveMessageReaction(_ context.Context, messageID, userID int64, emoji string) error {
	f.removed = append(f.removed, fakeReactionCall{messageID, userID, emoji})
	return nil
}

const allowedStamp = "stamp:thanks"

// newReactionService は本番の配線と同じく、権限判定をチャットの他サービスと共有する。
func newReactionService(h *harness) (MessageReactionService, *fakeReactions) {
	reactions := &fakeReactions{}
	svc := NewMessageReactionService(MessageReactionDeps{
		Access:     h.access,
		GetRoom:    &fakeGetRoom{rooms: testRooms()},
		GetMessage: h.getMessage,
		Reactions:  reactions,
		Events:     h.events,
	})
	return svc, reactions
}

func TestAddReaction_MemberCanReactToSomeoneElsesMessage(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	msg, err := svc.AddReaction(ctxAsUser(10), MessageReactionInput{RoomID: communityRoomID, MessageID: 5, Emoji: allowedStamp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil || msg.ID != 5 {
		t.Fatalf("expected the reacted message to be returned, got %+v", msg)
	}
	if len(reactions.added) != 1 || reactions.added[0] != (fakeReactionCall{5, 10, allowedStamp}) {
		t.Errorf("added = %+v, want one reaction by user 10 on message 5", reactions.added)
	}
	if len(h.events.updated) != 1 {
		t.Fatalf("expected one messageUpdated delivery, got %d", len(h.events.updated))
	}
	ev := h.events.updated[0]
	if ev.Room.ID != communityRoomID || ev.Message.ID != 5 {
		t.Errorf("event = room %d message %d, want room %d message 5", ev.Room.ID, ev.Message.ID, communityRoomID)
	}
	if ev.ActorID != 11 {
		t.Errorf("ActorID = %d, want the message author (11), not the person who reacted", ev.ActorID)
	}
	if len(ev.AddedMentions) != 0 {
		t.Error("a reaction must never trigger mention notifications")
	}
}

func TestAddReaction_CourseRoomWorksForEnrolledUser(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: courseRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	if _, err := svc.AddReaction(ctxAsUser(10), MessageReactionInput{RoomID: courseRoomID, MessageID: 5, Emoji: allowedStamp}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reactions.added) != 1 {
		t.Errorf("expected the reaction to be saved, got %+v", reactions.added)
	}
}

func TestAddReaction_RejectsEmojiOutsideTheAllowedList(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	for _, emoji := range []string{"", "🖕", "👍", "<script>"} {
		_, err := svc.AddReaction(ctxAsUser(10), MessageReactionInput{RoomID: communityRoomID, MessageID: 5, Emoji: emoji})
		if err == nil {
			t.Fatalf("emoji %q must be rejected", emoji)
		}
		if apperr.CodeOf(err) != apperr.CodeInvalidInput {
			t.Errorf("emoji %q: error code = %s, want %s", emoji, apperr.CodeOf(err), apperr.CodeInvalidInput)
		}
	}
	if len(reactions.added) != 0 || len(h.events.updated) != 0 {
		t.Error("a rejected emoji must not be saved or delivered")
	}
}

func TestAddReaction_RejectsMessageFromAnotherRoom(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	_, err := svc.AddReaction(ctxAsUser(10), MessageReactionInput{RoomID: courseRoomID, MessageID: 5, Emoji: allowedStamp})
	if err == nil {
		t.Fatal("expected a roomID/message mismatch to be rejected")
	}
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Errorf("error code = %s, want %s (存在の有無を漏らさないため not found に寄せる)", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if len(reactions.added) != 0 {
		t.Error("nothing must be saved when the roomID does not match")
	}
}

func TestAddReaction_NonMemberCannotReactInCommunity(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	// 12 は communityRoomID のメンバー（10, 11）に入っていない。
	if _, err := svc.AddReaction(ctxAsUser(12), MessageReactionInput{RoomID: communityRoomID, MessageID: 5, Emoji: allowedStamp}); err == nil {
		t.Fatal("expected a non-member to be rejected")
	}
	if len(reactions.added) != 0 || len(h.events.updated) != 0 {
		t.Error("a rejected reaction must not be saved or delivered")
	}
}

func TestAddReaction_ArchivedCourseRoomRejectsReaction(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: courseRoomID, UserID: 11}
	h.checkWritable.err = apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")
	svc, reactions := newReactionService(h)

	_, err := svc.AddReaction(ctxAsUser(10), MessageReactionInput{RoomID: courseRoomID, MessageID: 5, Emoji: allowedStamp})
	if err == nil {
		t.Fatal("expected a reaction in an archived course room to be rejected (編集・削除に揃えた)")
	}
	if apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Errorf("error code = %s, want %s", apperr.CodeOf(err), apperr.CodeForbidden)
	}
	if len(reactions.added) != 0 {
		t.Error("nothing must be saved when the course room is no longer writable")
	}
}

func TestAddReaction_RequiresLogin(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	if _, err := svc.AddReaction(context.Background(), MessageReactionInput{RoomID: communityRoomID, MessageID: 5, Emoji: allowedStamp}); err == nil {
		t.Fatal("expected an unauthenticated call to be rejected")
	}
	if len(reactions.added) != 0 {
		t.Error("nothing must be saved without a login")
	}
}

func TestRemoveReaction_RemovesOnlyTheCallersOwnReaction(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	if _, err := svc.RemoveReaction(ctxAsUser(10), MessageReactionInput{RoomID: communityRoomID, MessageID: 5, Emoji: allowedStamp}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reactions.removed) != 1 || reactions.removed[0] != (fakeReactionCall{5, 10, allowedStamp}) {
		t.Errorf("removed = %+v, want only user 10's own reaction", reactions.removed)
	}
	if len(h.events.updated) != 1 {
		t.Errorf("expected the new state to be delivered once, got %d", len(h.events.updated))
	}
}

func TestRemoveReaction_StillWorksForAnEmojiNoLongerOnTheList(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	// 許可リストから抜かれた絵文字でも、すでに付けた本人は外せる（外せないと残り続けてしまう）。
	if _, err := svc.RemoveReaction(ctxAsUser(10), MessageReactionInput{RoomID: communityRoomID, MessageID: 5, Emoji: "🙌"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reactions.removed) != 1 || reactions.removed[0] != (fakeReactionCall{5, 10, "🙌"}) {
		t.Errorf("removed = %+v, want user 10's own reaction", reactions.removed)
	}
}

func TestRemoveReaction_RejectsEmptyOrOverlongEmoji(t *testing.T) {
	h := newHarness()
	h.getMessage.msg = &model.Message{ID: 5, RoomID: communityRoomID, UserID: 11}
	svc, reactions := newReactionService(h)

	// 許可リストは見ないが、保存形式(VARCHAR(32))を超える長さや空文字はDBまで通さない。
	for _, emoji := range []string{"", strings.Repeat("あ", 33), strings.Repeat("x", 1<<20)} {
		_, err := svc.RemoveReaction(ctxAsUser(10), MessageReactionInput{RoomID: communityRoomID, MessageID: 5, Emoji: emoji})
		if err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
			t.Fatalf("emoji of %d bytes: err = %v, want InvalidInput", len(emoji), err)
		}
	}
	if len(reactions.removed) != 0 || len(h.events.updated) != 0 {
		t.Error("a rejected removal must not reach the repository or be delivered")
	}
}
