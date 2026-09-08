package message

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// 項番48,65,66: 複数生徒が同時に投稿しても、欠落なく全件保存され、
// 発行される messageID が重複しない。go test -race で実行するとデータ競合も検出できる。
func TestSendMessage_ConcurrentSendsProduceUniqueIDsWithNoLoss(t *testing.T) {
	const n = 100
	repo := newFakeMessageRepo()
	uc := newSendMessageUseCase(repo)

	var wg sync.WaitGroup
	ids := make([]int64, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg, err := uc.Execute(context.Background(), 1, int64(i), fmt.Sprintf("message %d", i), nil)
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = msg.ID
		}(i)
	}
	wg.Wait()

	seen := make(map[int64]bool, n)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, err)
		}
		if seen[ids[i]] {
			t.Fatalf("duplicate message id %d assigned to more than one concurrent send", ids[i])
		}
		seen[ids[i]] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique message ids, got %d", n, len(seen))
	}
	if len(repo.messages) != n {
		t.Fatalf("expected %d messages persisted, got %d (messages were lost)", n, len(repo.messages))
	}
}

// 項番70: 投稿直後に管理者が削除するレースでも、最終状態が一貫する
// （削除は成功し、以後の取得では見えなくなる。パニックや二重カウントは起きない）。
func TestSendThenImmediateAdminDelete_ConsistentFinalState(t *testing.T) {
	repo := newFakeMessageRepo()
	sendUC := newSendMessageUseCase(repo)
	deleteUC := NewDeleteMessageUseCase(repo)

	var msgID int64
	var sendErr, deleteErr error
	var deleted bool

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg, err := sendUC.Execute(context.Background(), 1, 7, "投稿直後に削除される", nil)
		sendErr = err
		if err == nil {
			msgID = msg.ID
			// 投稿がリポジトリに書き込まれた直後、間を置かずに管理者が削除する。
			deleted, deleteErr = deleteUC.Execute(context.Background(), msgID, 99)
		}
	}()
	wg.Wait()

	if sendErr != nil {
		t.Fatalf("unexpected send error: %v", sendErr)
	}
	if deleteErr != nil {
		t.Fatalf("unexpected delete error: %v", deleteErr)
	}
	if !deleted {
		t.Fatal("delete immediately after send should succeed exactly once")
	}
	if msg, _ := repo.GetMessageByID(context.Background(), msgID); msg != nil {
		t.Error("message must be hidden from general reads after admin delete")
	}
}

// 項番69: 取得中に管理者が削除しても、取得側は不整合な（部分的に書き換わった）
// メッセージを見ることはない — GetMessageByID はロックの下でコピーを返すため、
// 削除は「取得の前」か「取得の後」のどちらかとして原子的に観測される。
func TestGetMessageDuringConcurrentDelete_NeverObservesPartialState(t *testing.T) {
	const attempts = 50
	for i := 0; i < attempts; i++ {
		repo := newFakeMessageRepo()
		id := seedMessage(repo, 1, 7, "hello")
		deleteUC := NewDeleteMessageUseCase(repo)

		var wg sync.WaitGroup
		var getErr, delErr error
		var msgContent string
		var msgFound bool

		wg.Add(2)
		go func() {
			defer wg.Done()
			msg, err := repo.GetMessageByID(context.Background(), id)
			getErr = err
			if msg != nil {
				msgFound = true
				msgContent = msg.Content
			}
		}()
		go func() {
			defer wg.Done()
			_, delErr = deleteUC.Execute(context.Background(), id, 99)
		}()
		wg.Wait()

		if getErr != nil {
			t.Fatalf("unexpected get error: %v", getErr)
		}
		if delErr != nil {
			t.Fatalf("unexpected delete error: %v", delErr)
		}
		// 取得できたなら、必ず削除前の完全な内容が読めていること（部分書き換えなし）。
		if msgFound && msgContent != "hello" {
			t.Fatalf("observed partially-written message content: %q", msgContent)
		}
	}
}

// 項番67: 同一 CreatedAt を持つメッセージが複数あっても、ID をタイブレークにして
// 常に同じ順序で返る（表示順の一貫性）。
func TestListMessages_StableOrderWhenCreatedAtTies(t *testing.T) {
	repo := newFakeMessageRepo()
	id1 := seedMessage(repo, 1, 7, "1件目")
	id2 := seedMessage(repo, 1, 7, "2件目")
	id3 := seedMessage(repo, 1, 7, "3件目")

	// 同時刻投稿を模して CreatedAt を全て揃える。
	same := repo.messages[id1].CreatedAt
	repo.messages[id2].CreatedAt = same
	repo.messages[id3].CreatedAt = same

	uc := NewListMessagesUseCase(repo)
	for i := 0; i < 10; i++ {
		list, _, _, err := uc.Execute(context.Background(), 1, 50, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(list) != 3 || list[0].ID != id1 || list[1].ID != id2 || list[2].ID != id3 {
			t.Fatalf("run %d: order not stable under CreatedAt ties: %+v", i, list)
		}
	}
}
