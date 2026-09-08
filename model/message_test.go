package model

import (
	"testing"
	"time"
)

// CreateMessage は渡された CreateMessageParam の値をそのまま採用する。
// 呼び出し側（usecase/message.SendMessageInteractor）が常に time.Now() を
// CreatedAt/UpdatedAt に渡す設計であり、クライアントから時刻を差し込む経路は
// 型として存在しない（項番51,52,53の設計上の裏付け）。
func TestMessage_CreateMessage_UsesGivenTimestamps(t *testing.T) {
	m := &Message{}
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	m.CreateMessage(CreateMessageParam{
		RoomID:    1,
		UserID:    2,
		Content:   "hello",
		CreatedAt: created,
		UpdatedAt: created,
	})
	if !m.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", m.CreatedAt, created)
	}
	if !m.UpdatedAt.Equal(created) {
		t.Errorf("UpdatedAt = %v, want %v", m.UpdatedAt, created)
	}
}

// UpdateMessage は Content が指定された場合のみ書き換え、UpdatedAt は常に
// 更新時点のサーバー時刻になる（クライアントが updatedAt を指定する経路はない）。
func TestMessage_UpdateMessage_SetsUpdatedAtToNow(t *testing.T) {
	m := &Message{Content: "original", UpdatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	before := time.Now()

	newContent := "edited"
	m.UpdateMessage(UpdateMessageParam{Content: &newContent})

	if m.Content != "edited" {
		t.Errorf("Content = %q, want %q", m.Content, "edited")
	}
	if m.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt = %v, want it to be set to now (>= %v)", m.UpdatedAt, before)
	}
}

func TestMessage_UpdateMessage_NilContentLeavesContentUnchanged(t *testing.T) {
	m := &Message{Content: "original"}
	m.UpdateMessage(UpdateMessageParam{Content: nil})
	if m.Content != "original" {
		t.Errorf("Content = %q, want unchanged %q", m.Content, "original")
	}
}

func TestMessage_IsDeleted(t *testing.T) {
	m := &Message{}
	if m.IsDeleted() {
		t.Error("new message should not be deleted")
	}
	now := time.Now()
	m.DeletedAt = &now
	if !m.IsDeleted() {
		t.Error("message with DeletedAt set should be deleted")
	}
}
