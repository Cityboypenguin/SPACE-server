package model

import (
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

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
