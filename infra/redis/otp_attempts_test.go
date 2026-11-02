package redis

import (
	"context"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newOTPTestClient(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// 間違いが続くと、正しいコードでも通らなくなる。取り直せばまた試せる。
func TestPasswordResetOTP_DiscardedAfterFailures(t *testing.T) {
	repo := NewRedisPasswordResetRepository(newOTPTestClient(t))
	ctx := context.Background()
	const email = "A@example.com"
	if err := repo.SaveOTP(ctx, email, "123456", time.Minute); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < otpMaxFailures-1; i++ {
		if ok, err := repo.ExchangeOTPForResetToken(ctx, email, "000000", "t", time.Minute); err != nil || ok {
			t.Fatalf("wrong code %d: ok=%v err=%v", i, ok, err)
		}
	}
	if err := repo.SaveOTP(ctx, email, "123456", time.Minute); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < otpMaxFailures; i++ {
		_, _ = repo.ExchangeOTPForResetToken(ctx, email, "000000", "t", time.Minute)
	}
	if ok, err := repo.ExchangeOTPForResetToken(ctx, email, "123456", "t", time.Minute); err != nil || ok {
		t.Fatalf("the correct code after too many failures: ok=%v err=%v", ok, err)
	}
	if err := repo.SaveOTP(ctx, email, "654321", time.Minute); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.ExchangeOTPForResetToken(ctx, "a@example.com", "654321", "t", time.Minute); err != nil || !ok {
		t.Fatalf("a new code: ok=%v err=%v", ok, err)
	}
}

func TestPasswordResetOTP_CorrectCodeBelowLimitWorks(t *testing.T) {
	repo := NewRedisPasswordResetRepository(newOTPTestClient(t))
	ctx := context.Background()
	if err := repo.SaveOTP(ctx, "a@example.com", "123456", time.Minute); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < otpMaxFailures-1; i++ {
		_, _ = repo.ExchangeOTPForResetToken(ctx, "a@example.com", "000000", "t", time.Minute)
	}
	if ok, err := repo.ExchangeOTPForResetToken(ctx, "a@example.com", "123456", "t", time.Minute); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestEmailOTP_DiscardedAfterFailures(t *testing.T) {
	repo := NewRedisEmailOTPRepository(newOTPTestClient(t))
	ctx := context.Background()
	if err := repo.Save(ctx, &model.EmailOTP{Email: "a@example.com", Code: "123456"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < otpMaxFailures-1; i++ {
		if err := repo.RecordFailure(ctx, "a@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if rec, _ := repo.FindLatestByEmail(ctx, "a@example.com"); rec == nil {
		t.Fatal("discarded before the limit")
	}
	if err := repo.RecordFailure(ctx, "a@example.com"); err != nil {
		t.Fatal(err)
	}
	if rec, _ := repo.FindLatestByEmail(ctx, "a@example.com"); rec != nil {
		t.Fatal("still usable after the limit")
	}
}
