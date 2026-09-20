package notification

import (
	"context"
	"encoding/base64"
	"testing"
)

type stubTicketRepo struct {
	issued map[string]int64
	err    error
}

func (s *stubTicketRepo) Issue(_ context.Context, ticket string, userID int64) error {
	if s.err != nil {
		return s.err
	}
	s.issued[ticket] = userID
	return nil
}

func (s *stubTicketRepo) Consume(_ context.Context, ticket string) (int64, bool, error) {
	userID, ok := s.issued[ticket]
	if !ok {
		return 0, false, nil
	}
	delete(s.issued, ticket)
	return userID, true, nil
}

// 発行したチケットが URL に直接載せられる形（RawURLEncoding = +/= が出ない）で、
// 呼び出したユーザーに紐づいて保存されること。
func TestIssueStreamTicket_IssuesURLSafeTicketForCaller(t *testing.T) {
	repo := &stubTicketRepo{issued: map[string]int64{}}
	uc := NewIssueStreamTicketUseCase(repo)

	ticket, err := uc.Execute(context.Background(), 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticket == "" {
		t.Fatal("ticket must not be empty")
	}
	raw, err := base64.RawURLEncoding.DecodeString(ticket)
	if err != nil {
		t.Fatalf("ticket is not RawURLEncoding base64 (would need escaping in a URL): %v", err)
	}
	if len(raw) != streamTicketBytes {
		t.Fatalf("expected %d bytes of entropy, got %d", streamTicketBytes, len(raw))
	}
	if repo.issued[ticket] != 123 {
		t.Fatalf("ticket should be bound to caller 123, got %d", repo.issued[ticket])
	}
}

// 毎回違うチケットが出ること（使い捨てである以上、再発行で同じ値が出ては困る）。
func TestIssueStreamTicket_IsUniquePerCall(t *testing.T) {
	repo := &stubTicketRepo{issued: map[string]int64{}}
	uc := NewIssueStreamTicketUseCase(repo)

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		ticket, err := uc.Execute(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[ticket] {
			t.Fatalf("duplicate ticket generated: %s", ticket)
		}
		seen[ticket] = true
	}
}

// 保存に失敗したらエラーを返すこと（「発行できたがどこにも無いチケット」を
// 渡してしまうと、クライアントは必ず 401 になる接続を試みることになる）。
func TestIssueStreamTicket_StoreFailure(t *testing.T) {
	repo := &stubTicketRepo{issued: map[string]int64{}, err: context.DeadlineExceeded}
	uc := NewIssueStreamTicketUseCase(repo)

	if _, err := uc.Execute(context.Background(), 1); err == nil {
		t.Fatal("expected error when the ticket store fails")
	}
}
