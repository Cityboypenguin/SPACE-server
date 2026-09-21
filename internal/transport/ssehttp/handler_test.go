package ssehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// fakeTicketRepo は repository.SSETicketRepository のインメモリ実装。
// Consume が「取得と削除を不可分に行う」という契約（Redis の GETDEL に対応）を
// mutex で再現する。使い回しが弾かれることを検証できるのはこの性質による。
type fakeTicketRepo struct {
	mu      sync.Mutex
	tickets map[string]repository.StreamSession
	err     error
}

func newFakeTicketRepo() *fakeTicketRepo {
	return &fakeTicketRepo{tickets: map[string]repository.StreamSession{}}
}

func (f *fakeTicketRepo) Issue(_ context.Context, ticket string, session repository.StreamSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tickets[ticket] = session
	return nil
}

func (f *fakeTicketRepo) Consume(_ context.Context, ticket string) (repository.StreamSession, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return repository.StreamSession{}, false, f.err
	}
	session, ok := f.tickets[ticket]
	if !ok {
		return repository.StreamSession{}, false, nil
	}
	delete(f.tickets, ticket) // 1回使ったら消える
	return session, true, nil
}

var _ repository.SSETicketRepository = (*fakeTicketRepo)(nil)

// authCtx は authenticate() だけを呼ぶための最小の echo.Context を組む。
// ハンドラ本体は接続を張りっぱなしにするため、認証部分だけを切り出して検証する。
func authCtx(t *testing.T, rawQuery string) echo.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/events?"+rawQuery, nil)
	return echo.New().NewContext(req, httptest.NewRecorder())
}

func httpStatus(t *testing.T, err error) int {
	t.Helper()
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T (%v)", err, err)
	}
	return he.Code
}

// 有効なチケットで認証が通り、発行時の userID が返ること。
func TestAuthenticate_ValidTicket(t *testing.T) {
	repo := newFakeTicketRepo()
	if err := repo.Issue(context.Background(), "tkt-abc", repository.StreamSession{UserID: 42, Token: "tok-42"}); err != nil {
		t.Fatalf("issue: %v", err)
	}

	session, err := authenticate(authCtx(t, "ticket=tkt-abc"), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.UserID != 42 {
		t.Fatalf("expected userID 42, got %d", session.UserID)
	}
	// トークンも一緒に引き換わること。これが無いと、接続したあとに
	// 認証をやり直す手立てが無くなる。
	if session.Token != "tok-42" {
		t.Fatalf("expected the access token to come back with the ticket, got %q", session.Token)
	}
}

// 同じチケットの2回目が弾かれること（使い捨ての保証）。
// アクセスログに残ったチケットを拾われても繋がらない、という性質そのもの。
func TestAuthenticate_TicketIsSingleUse(t *testing.T) {
	repo := newFakeTicketRepo()
	if err := repo.Issue(context.Background(), "tkt-once", repository.StreamSession{UserID: 7, Token: "tok-7"}); err != nil {
		t.Fatalf("issue: %v", err)
	}

	if _, err := authenticate(authCtx(t, "ticket=tkt-once"), repo); err != nil {
		t.Fatalf("first use should succeed: %v", err)
	}

	_, err := authenticate(authCtx(t, "ticket=tkt-once"), repo)
	if err == nil {
		t.Fatal("second use of the same ticket must be rejected")
	}
	if code := httpStatus(t, err); code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

// 期限切れ（＝ストアに無い）チケットが弾かれること。
// Redis の TTL 失効はキーが消えることなので、未知のチケットと同じ扱いになる。
func TestAuthenticate_ExpiredOrUnknownTicket(t *testing.T) {
	_, err := authenticate(authCtx(t, "ticket=never-issued"), newFakeTicketRepo())
	if err == nil {
		t.Fatal("unknown ticket must be rejected")
	}
	if code := httpStatus(t, err); code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

// チケットもヘッダー認証も無ければ 401。
func TestAuthenticate_NoCredentials(t *testing.T) {
	_, err := authenticate(authCtx(t, ""), newFakeTicketRepo())
	if err == nil {
		t.Fatal("request without credentials must be rejected")
	}
	if code := httpStatus(t, err); code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

// ストア障害は 401（無効なチケット）ではなく 500 にする。
// 「チケットが無効」と「検証できなかった」を混ぜると、Redis 障害時に
// クライアントが「ログインし直せ」と誤解する挙動になるため。
func TestAuthenticate_TicketStoreFailure(t *testing.T) {
	repo := newFakeTicketRepo()
	repo.err = context.DeadlineExceeded

	_, err := authenticate(authCtx(t, "ticket=whatever"), repo)
	if err == nil {
		t.Fatal("store failure must not authenticate")
	}
	if code := httpStatus(t, err); code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", code)
	}
}

// ヘッダー認証済み（middleware がクレームを載せている）ならチケットは要らないこと。
func TestAuthenticate_HeaderClaimsTakePrecedence(t *testing.T) {
	repo := newFakeTicketRepo()
	c := authCtx(t, "")
	req := c.Request()
	ctx := auth.WithClaims(req.Context(), &auth.Claims{ID: 99})
	ctx = auth.WithToken(ctx, "tok-99")
	c.SetRequest(req.WithContext(ctx))

	session, err := authenticate(c, repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.UserID != 99 {
		t.Fatalf("expected userID 99, got %d", session.UserID)
	}
	if session.Token != "tok-99" {
		t.Fatalf("expected the middleware's token to come through, got %q", session.Token)
	}
}

func TestAuthenticate_RejectsLegacyQueryToken(t *testing.T) {
	_, err := authenticate(authCtx(t, "token=legacy-jwt"), newFakeTicketRepo())
	if err == nil {
		t.Fatal("legacy query token must be rejected")
	}
	if code := httpStatus(t, err); code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}
