package message

import (
	"context"
	"testing"
)

func seedMessage(repo *fakeMessageRepo, roomID, userID int64, content string) int64 {
	uc := newSendMessageUseCase(repo)
	msg, err := uc.Execute(context.Background(), roomID, userID, content, nil)
	if err != nil {
		panic(err)
	}
	return msg.ID
}

// 項番41,54: 管理者が削除すると論理削除され、deletedAt/deletedBy が正しく設定される。
func TestDeleteMessage_SoftDeletesAndRecordsActor(t *testing.T) {
	repo := newFakeMessageRepo()
	id := seedMessage(repo, 1, 7, "不適切な投稿")
	uc := NewDeleteMessageUseCase(repo)

	const adminID = int64(99)
	deleted, err := uc.Execute(context.Background(), id, adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Fatal("expected deleted = true")
	}

	stored := repo.getMessageByIDIncludingDeleted(id)
	if stored == nil {
		t.Fatal("message row should still exist after soft delete")
	}
	if !stored.IsDeleted() {
		t.Error("DeletedAt should be set")
	}
	if stored.DeletedBy == nil || *stored.DeletedBy != adminID {
		t.Errorf("DeletedBy = %v, want %d", stored.DeletedBy, adminID)
	}
}

// 項番55: 削除済みメッセージは一般画面(GetMessageByID/一覧)から見えなくなる。
func TestDeleteMessage_HidesFromGeneralReads(t *testing.T) {
	repo := newFakeMessageRepo()
	id := seedMessage(repo, 1, 7, "hello")
	uc := NewDeleteMessageUseCase(repo)
	if _, err := uc.Execute(context.Background(), id, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg, err := repo.GetMessageByID(context.Background(), id); err != nil || msg != nil {
		t.Errorf("GetMessageByID should hide deleted message, got msg=%v err=%v", msg, err)
	}

	list, _, _, err := NewListMessagesUseCase(repo).Execute(context.Background(), 1, 50, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, m := range list {
		if m.ID == id {
			t.Error("deleted message must not appear in room listing")
		}
	}
}

// 項番39: 存在しない投稿IDを削除 -> 404相当（deleted=false, error）になる。
func TestDeleteMessage_NonexistentIDReturnsNotDeleted(t *testing.T) {
	repo := newFakeMessageRepo()
	uc := NewDeleteMessageUseCase(repo)

	deleted, err := uc.Execute(context.Background(), 12345, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted {
		t.Fatal("deleting a nonexistent message must report deleted = false")
	}
}

// 項番40: 既に削除済みの投稿を再削除しても、二重削除でデータ不整合が起きない
// （2回目の削除者で deletedBy が上書きされない）。
func TestDeleteMessage_DoubleDeleteIsIdempotent(t *testing.T) {
	repo := newFakeMessageRepo()
	id := seedMessage(repo, 1, 7, "hello")
	uc := NewDeleteMessageUseCase(repo)

	const firstDeleter = int64(99)
	const secondDeleter = int64(100)

	firstResult, err := uc.Execute(context.Background(), id, firstDeleter)
	if err != nil || !firstResult {
		t.Fatalf("first delete should succeed: deleted=%v err=%v", firstResult, err)
	}

	secondResult, err := uc.Execute(context.Background(), id, secondDeleter)
	if err != nil {
		t.Fatalf("unexpected error on second delete: %v", err)
	}
	if secondResult {
		t.Fatal("second delete of an already-deleted message must report deleted = false")
	}

	stored := repo.getMessageByIDIncludingDeleted(id)
	if stored.DeletedBy == nil || *stored.DeletedBy != firstDeleter {
		t.Errorf("DeletedBy must remain the first deleter (%d), got %v", firstDeleter, stored.DeletedBy)
	}
}
