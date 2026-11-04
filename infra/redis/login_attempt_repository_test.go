package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestLoginAttemptRepository_LocksAtLimitAndExpires(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	repo := NewRedisLoginAttemptRepository(client)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := repo.RecordFailure(ctx, "k", 3, time.Minute, 5*time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if locked, _ := repo.IsLocked(ctx, "k"); locked {
		t.Fatal("locked before reaching the limit")
	}
	if err := repo.RecordFailure(ctx, "k", 3, time.Minute, 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	if locked, _ := repo.IsLocked(ctx, "k"); !locked {
		t.Fatal("not locked at the limit")
	}
	if locked, _ := repo.IsLocked(ctx, "other"); locked {
		t.Fatal("another key was locked")
	}

	srv.FastForward(5*time.Minute + time.Second)
	if locked, _ := repo.IsLocked(ctx, "k"); locked {
		t.Fatal("still locked after the lock expired")
	}
}

// 失敗の数は window で消えるので、間を空けた打ち間違いでは止まらない。
func TestLoginAttemptRepository_FailuresExpireWithWindow(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	repo := NewRedisLoginAttemptRepository(client)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_ = repo.RecordFailure(ctx, "k", 3, time.Minute, 5*time.Minute)
	}
	srv.FastForward(time.Minute + time.Second)
	_ = repo.RecordFailure(ctx, "k", 3, time.Minute, 5*time.Minute)
	if locked, _ := repo.IsLocked(ctx, "k"); locked {
		t.Fatal("failures outside the window were counted")
	}
}
