package loginguard

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memRepo struct {
	failures map[string]int
	locked   map[string]bool
	readErr  error
}

func newMemRepo() *memRepo {
	return &memRepo{failures: map[string]int{}, locked: map[string]bool{}}
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

func TestAttempt_SuccessClearsFailures(t *testing.T) {
	repo := newMemRepo()
	g := New(repo)
	bad := func() (int, error) { return 0, ErrInvalidCredentials }
	for i := 0; i < MaxFailures-1; i++ {
		_, _ = Attempt(context.Background(), g, "user", "a@example.com", bad)
	}
	if _, err := Attempt(context.Background(), g, "user", "a@example.com", func() (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	if repo.failures["user:a@example.com"] != 0 {
		t.Fatal("success did not clear the failures")
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
