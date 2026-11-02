package loginguard

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/clientip"
)

type memRepo struct {
	failures map[string]int
	locked   map[string]bool
	known    map[string]bool
	readErr  error
}

func newMemRepo() *memRepo {
	return &memRepo{failures: map[string]int{}, locked: map[string]bool{}, known: map[string]bool{}}
}

func (r *memRepo) RememberIP(_ context.Context, key, ip string, _ time.Duration) error {
	r.known[key+"|"+ip] = true
	return nil
}

func (r *memRepo) IsKnownIP(_ context.Context, key, ip string) (bool, error) {
	return r.known[key+"|"+ip], r.readErr
}

func (r *memRepo) IsLocked(_ context.Context, key string) (bool, error) {
	return r.locked[key], r.readErr
}

func (r *memRepo) RecordFailure(_ context.Context, key string, maxFailures int, _, _ time.Duration) error {
	r.failures[key]++
	if r.failures[key] >= maxFailures {
		r.locked[key] = true
	}
	return nil
}

func (r *memRepo) Clear(_ context.Context, key string) error {
	delete(r.failures, key)
	delete(r.locked, key)
	return nil
}

func TestAttempt_LocksAfterMaxFailuresWithoutRunningLogin(t *testing.T) {
	g, calls := New(newMemRepo()), 0
	login := func() (int, error) { calls++; return 0, ErrInvalidCredentials }
	for i := 0; i < MaxFailures; i++ {
		if _, err := Attempt(context.Background(), g, "user", "a@example.com", login); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := Attempt(context.Background(), g, "user", " A@Example.com ", login); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("want ErrTooManyAttempts, got %v", err)
	}
	if calls != MaxFailures {
		t.Fatalf("login ran %d times, want %d", calls, MaxFailures)
	}
	// 管理者と利用者、別のアカウントは巻き込まない。
	for _, c := range [][2]string{{"admin", "a@example.com"}, {"user", "b@example.com"}} {
		if _, err := Attempt(context.Background(), g, c[0], c[1], login); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("%v was affected: %v", c, err)
		}
	}
}

func TestAttempt_SuccessClearsFailuresFromThatIP(t *testing.T) {
	repo := newMemRepo()
	g := New(repo)
	ctx := clientip.WithIP(context.Background(), "203.0.113.5")
	bad := func() (int, error) { return 0, ErrInvalidCredentials }
	for i := 0; i < MaxFailures-1; i++ {
		_, _ = Attempt(ctx, g, "user", "a@example.com", bad)
	}
	if _, err := Attempt(ctx, g, "user", "a@example.com", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	if repo.failures["user:a@example.com|203.0.113.5"] != 0 {
		t.Fatal("success did not clear the failures from that ip")
	}
	if !repo.known["user:a@example.com|203.0.113.5"] {
		t.Fatal("success did not remember the ip")
	}
}

// 第三者が別の IP から失敗を重ねても、本人は以前ログインできた IP からならログインできる。
func TestAttempt_LockByOthersDoesNotBlockKnownIP(t *testing.T) {
	g := New(newMemRepo())
	owner := clientip.WithIP(context.Background(), "203.0.113.5")
	if _, err := Attempt(owner, g, "user", "a@example.com", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	bad := func() (int, error) { return 0, ErrInvalidCredentials }
	for i := 0; i < MaxFailures; i++ {
		_, _ = Attempt(clientip.WithIP(context.Background(), fmt.Sprintf("198.51.100.%d", i)), g, "user", "a@example.com", bad)
	}
	// IP を変えながらの試行は、アカウント全体の回数で止まる。
	if _, err := Attempt(clientip.WithIP(context.Background(), "198.51.100.200"), g, "user", "a@example.com", bad); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("unknown ip: want ErrTooManyAttempts, got %v", err)
	}
	if _, err := Attempt(owner, g, "user", "a@example.com", func() (int, error) { return 1, nil }); err != nil {
		t.Fatalf("known ip was locked out: %v", err)
	}
}

// 本人の IP からでも、そこからの総当たりは止まる。
func TestAttempt_KnownIPIsStillLimited(t *testing.T) {
	g := New(newMemRepo())
	ctx := clientip.WithIP(context.Background(), "203.0.113.5")
	if _, err := Attempt(ctx, g, "user", "a@example.com", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	bad := func() (int, error) { return 0, ErrInvalidCredentials }
	for i := 0; i < MaxFailures; i++ {
		_, _ = Attempt(ctx, g, "user", "a@example.com", bad)
	}
	if _, err := Attempt(ctx, g, "user", "a@example.com", bad); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("want ErrTooManyAttempts, got %v", err)
	}
}

// 凍結や DB の失敗はパスワードの総当たりではないので数えない。
func TestAttempt_OtherErrorsAreNotCounted(t *testing.T) {
	repo := newMemRepo()
	want := errors.New("account is frozen")
	if _, err := Attempt(context.Background(), New(repo), "user", "a@example.com", func() (int, error) { return 0, want }); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
	if repo.failures["user:a@example.com"] != 0 {
		t.Fatal("an unrelated error was counted")
	}
}

func TestAttempt_RedisFailureDoesNotBlockLogin(t *testing.T) {
	repo := newMemRepo()
	repo.readErr = errors.New("redis down")
	got, err := Attempt(context.Background(), New(repo), "user", "a@example.com", func() (int, error) { return 7, nil })
	if err != nil || got != 7 {
		t.Fatalf("got %d, %v", got, err)
	}
}

func TestForgive_ClearsFailuresAndRemembersIP(t *testing.T) {
	repo := newMemRepo()
	repo.failures["user:a@example.com"] = 3
	repo.failures["user:a@example.com|203.0.113.5"] = 3
	New(repo).Forgive(clientip.WithIP(context.Background(), "203.0.113.5"), ScopeUser, " A@example.com ")
	if repo.failures["user:a@example.com"] != 0 || repo.failures["user:a@example.com|203.0.113.5"] != 0 {
		t.Fatal("forgive did not clear the failures")
	}
	if !repo.known["user:a@example.com|203.0.113.5"] {
		t.Fatal("forgive did not remember the ip")
	}
}
