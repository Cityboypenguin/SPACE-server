package message

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// 存在しないメッセージIDを更新しようとしても nil ポインタ参照でパニックせず、
// エラーを返す（旧実装は message が nil のまま UpdateMessage を呼びパニックしていた）。
func TestUpdateMessage_NonexistentIDReturnsErrorNotPanic(t *testing.T) {
	repo := newFakeMessageRepo()
	uc := NewUpdateMessageUseCase(repo)
	content := "new content"

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("must not panic for nonexistent message id, got panic: %v", r)
		}
	}()

	if _, err := uc.Execute(context.Background(), 12345, model.UpdateMessageParam{Content: &content}); err == nil {
		t.Fatal("expected error for nonexistent message id")
	}
}

// 最大文字数超過の内容には更新できない。
func TestUpdateMessage_RejectsContentOverMaxLength(t *testing.T) {
	repo := newFakeMessageRepo()
	id := seedMessage(repo, 1, 7, "original")
	uc := NewUpdateMessageUseCase(repo)

	tooLong := make([]rune, MaxContentChars+1)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	content := string(tooLong)

	if _, err := uc.Execute(context.Background(), id, model.UpdateMessageParam{Content: &content}); err == nil {
		t.Fatal("expected error for content exceeding max length")
	}

	stored := repo.getMessageByIDIncludingDeleted(id)
	if stored.Content != "original" {
		t.Errorf("content should be unchanged after rejected update, got %q", stored.Content)
	}
}

// 削除済みメッセージは更新できない（内容が復活しない）。
func TestUpdateMessage_DeletedMessageCannotBeUpdated(t *testing.T) {
	repo := newFakeMessageRepo()
	id := seedMessage(repo, 1, 7, "original")
	if _, err := NewDeleteMessageUseCase(repo).Execute(context.Background(), id, 99); err != nil {
		t.Fatalf("setup: unexpected delete error: %v", err)
	}

	uc := NewUpdateMessageUseCase(repo)
	content := "edited after delete"
	if _, err := uc.Execute(context.Background(), id, model.UpdateMessageParam{Content: &content}); err == nil {
		t.Fatal("expected error updating a deleted message")
	}
}
