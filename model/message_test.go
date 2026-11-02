package model

import (
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

func TestNewMessage_RequiresContentOrMedia(t *testing.T) {
	if _, err := NewMessage(CreateMessageParam{RoomID: 1, UserID: 2, Content: "   "}); err == nil {
		t.Fatal("本文も添付も無いメッセージは作れないはず")
	} else if apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("code = %s, want %s", apperr.CodeOf(err), apperr.CodeInvalidInput)
	}

	// 本文が空でも添付があれば成立する。
	m, err := NewMessage(CreateMessageParam{RoomID: 1, UserID: 2, MediaKeys: []string{"media/2/a.png"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Content != "" {
		t.Fatalf("content = %q, want empty", m.Content)
	}
}

func TestNewMessage_DefaultsAuthorRoleAndTimestamps(t *testing.T) {
	m, err := NewMessage(CreateMessageParam{RoomID: 1, UserID: 2, Content: " hello "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.AuthorRole != AuthorRoleStudent {
		t.Fatalf("authorRole = %q, want %q", m.AuthorRole, AuthorRoleStudent)
	}
	if m.Content != "hello" {
		t.Fatalf("content = %q, want trimmed", m.Content)
	}
	if m.CreatedAt.IsZero() || !m.UpdatedAt.Equal(m.CreatedAt) {
		t.Fatalf("createdAt=%v updatedAt=%v, want both set to the same time", m.CreatedAt, m.UpdatedAt)
	}

	m, err = NewMessage(CreateMessageParam{RoomID: 1, UserID: 2, Content: "hi", AuthorRole: AuthorRoleTeacher})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.AuthorRole != AuthorRoleTeacher {
		t.Fatalf("authorRole = %q, want %q", m.AuthorRole, AuthorRoleTeacher)
	}
}

func TestMessagePermissions(t *testing.T) {
	m := &Message{ID: 1, RoomID: 7, UserID: 2}

	if !m.IsInRoom(7) || m.IsInRoom(8) {
		t.Fatal("IsInRoom はルームIDの一致をそのまま返すはず")
	}
	if !m.CanBeEditedBy(2, false) {
		t.Fatal("本人は編集できるはず")
	}
	if m.CanBeEditedBy(3, false) {
		t.Fatal("他人は編集できないはず")
	}
	if !m.CanBeDeletedBy(3, true) {
		t.Fatal("管理者は削除できるはず")
	}

	deletedAt := time.Now()
	m.DeletedAt = &deletedAt
	if m.CanBeEditedBy(2, true) || m.CanBeDeletedBy(2, true) {
		t.Fatal("削除済みのメッセージは本人でも管理者でも触れないはず")
	}
}

func strPtr(s string) *string { return &s }

func TestNewMessage_StampOnly(t *testing.T) {
	m, err := NewMessage(CreateMessageParam{RoomID: 1, UserID: 2, StampID: strPtr("stamp:ryokai")})
	if err != nil {
		t.Fatalf("a stamp-only message must be accepted, got: %v", err)
	}
	if m.StampID == nil || *m.StampID != "stamp:ryokai" || m.Content != "" {
		t.Errorf("message = %+v, want the stamp kept and no content", m)
	}
}

func TestNewMessage_StampCannotBeCombined(t *testing.T) {
	cases := map[string]CreateMessageParam{
		"with content": {RoomID: 1, UserID: 2, Content: "こんにちは", StampID: strPtr("stamp:ryokai")},
		"with media":   {RoomID: 1, UserID: 2, MediaKeys: []string{"k"}, StampID: strPtr("stamp:ryokai")},
	}
	for name, param := range cases {
		if _, err := NewMessage(param); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
			t.Errorf("%s: err = %v, want InvalidInput", name, err)
		}
	}
}

func TestNewMessage_RejectsStampsNotOnTheList(t *testing.T) {
	for _, id := range []string{"", "stamp:", "stamp:unknown", "👍", "ryokai"} {
		if _, err := NewMessage(CreateMessageParam{RoomID: 1, UserID: 2, StampID: strPtr(id)}); err == nil {
			t.Errorf("stamp id %q must be rejected", id)
		}
	}
}

func TestNewMessage_StillRequiresContentOrMediaWithoutAStamp(t *testing.T) {
	if _, err := NewMessage(CreateMessageParam{RoomID: 1, UserID: 2, Content: "  "}); err == nil {
		t.Error("an empty message without a stamp must be rejected")
	}
}
