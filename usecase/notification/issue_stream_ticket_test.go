package notification

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type stubTicketRepo struct {
	issued map[string]repository.StreamSession
	err    error
}

func (s *stubTicketRepo) Issue(_ context.Context, ticket string, session repository.StreamSession) error {
	if s.err != nil {
		return s.err
	}
	s.issued[ticket] = session
	return nil
}

func (s *stubTicketRepo) Consume(_ context.Context, ticket string) (repository.StreamSession, bool, error) {
	session, ok := s.issued[ticket]
	if !ok {
		return repository.StreamSession{}, false, nil
	}
	delete(s.issued, ticket)
	return session, true, nil
}

// 発行したチケットが URL に直接載せられる形（RawURLEncoding = +/= が出ない）で、
// 呼び出したユーザーに紐づいて保存されること。
func TestIssueStreamTicket_IssuesURLSafeTicketForCaller(t *testing.T) {
	repo := &stubTicketRepo{issued: map[string]repository.StreamSession{}}
	uc := NewIssueStreamTicketUseCase(repo)

	ticket, err := uc.Execute(authz_ctx(123), "tok")
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
	if repo.issued[ticket].UserID != 123 {
		t.Fatalf("ticket should be bound to caller 123, got %d", repo.issued[ticket].UserID)
	}
	// トークンも預かること。/events は接続後に認証をやり直すので、
	// userID だけを預かるとログアウトしてもストリームが残る。
	if repo.issued[ticket].Token != "tok" {
		t.Fatalf("ticket should carry the access token, got %q", repo.issued[ticket].Token)
	}
}

// 毎回違うチケットが出ること（使い捨てである以上、再発行で同じ値が出ては困る）。
func TestIssueStreamTicket_IsUniquePerCall(t *testing.T) {
	repo := &stubTicketRepo{issued: map[string]repository.StreamSession{}}
	uc := NewIssueStreamTicketUseCase(repo)

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		ticket, err := uc.Execute(authz_ctx(1), "tok")
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
	repo := &stubTicketRepo{issued: map[string]repository.StreamSession{}, err: context.DeadlineExceeded}
	uc := NewIssueStreamTicketUseCase(repo)

	if _, err := uc.Execute(authz_ctx(1), "tok"); err == nil {
		t.Fatal("expected error when the ticket store fails")
	}
}

// authz_ctx は「この利用者として呼ばれた」ctx を作る。
// 行為者は引数ではなく ctx から決まるようになったので、テストもそこへ置く。
func authz_ctx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID})
}
