package message

import (
	"context"
	"testing"
)

// 項番13: 複数投稿が作成順（ID昇順）で一貫して表示される。
func TestListMessages_ReturnsInIDOrder(t *testing.T) {
	repo := newFakeMessageRepo()
	id1 := seedMessage(repo, 1, 7, "1件目")
	id2 := seedMessage(repo, 1, 7, "2件目")
	id3 := seedMessage(repo, 1, 7, "3件目")

	uc := NewListMessagesUseCase(repo)
	list, _, _, err := uc.Execute(context.Background(), 1, 50, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(list))
	}
	wantOrder := []int64{id1, id2, id3}
	for i, want := range wantOrder {
		if list[i].ID != want {
			t.Errorf("position %d: got message id %d, want %d", i, list[i].ID, want)
		}
	}
}

// 項番12: 投稿が0件の授業では空のスライスが返る（nilでもエラーでもない）。
func TestListMessages_EmptyRoomReturnsNoMessages(t *testing.T) {
	repo := newFakeMessageRepo()
	seedMessage(repo, 999, 7, "別の授業の投稿")

	uc := NewListMessagesUseCase(repo)
	list, _, _, err := uc.Execute(context.Background(), 1, 50, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no messages for an empty room, got %d", len(list))
	}
}

// 項番11: 対象授業のメッセージのみが返り、他授業のメッセージは混入しない。
func TestListMessages_OnlyReturnsMessagesForRequestedRoom(t *testing.T) {
	repo := newFakeMessageRepo()
	idInRoomA := seedMessage(repo, 1, 7, "授業Aの投稿")
	seedMessage(repo, 2, 7, "授業Bの投稿")

	uc := NewListMessagesUseCase(repo)
	list, _, _, err := uc.Execute(context.Background(), 1, 50, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 || list[0].ID != idInRoomA {
		t.Fatalf("expected only room A's message, got %+v", list)
	}
}
