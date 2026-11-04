package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestTermsClaim(t *testing.T, srv *miniredis.Miniredis) *TermsBroadcastClaim {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewTermsBroadcastClaim(client, "test:")
}

// 台を2つ立てても、同じ版を配れるのは片方だけ。
func TestTermsBroadcastClaim_OnlyOneInstanceWins(t *testing.T) {
	srv := miniredis.RunT(t)
	c1, c2 := newTestTermsClaim(t, srv), newTestTermsClaim(t, srv)
	ctx := context.Background()

	got1, err := c1.Claim(ctx, "v1")
	if err != nil {
		t.Fatalf("Claim returned an error: %v", err)
	}
	got2, err := c2.Claim(ctx, "v1")
	if err != nil {
		t.Fatalf("Claim returned an error: %v", err)
	}

	if !got1 || got2 {
		t.Fatalf("claims = (%v, %v), want exactly the first one to win", got1, got2)
	}
}

// 再起動を跨いでも1回だけ。印を消さないので2度目は取れない。
func TestTermsBroadcastClaim_StaysClaimedAcrossRestarts(t *testing.T) {
	srv := miniredis.RunT(t)
	ctx := context.Background()

	if ok, _ := newTestTermsClaim(t, srv).Claim(ctx, "v2"); !ok {
		t.Fatal("the first claim must win")
	}
	if ok, _ := newTestTermsClaim(t, srv).Claim(ctx, "v2"); ok {
		t.Fatal("a later boot must not re-broadcast a version already sent")
	}
}

func TestTermsBroadcastClaim_TracksVersionsSeparately(t *testing.T) {
	srv := miniredis.RunT(t)
	c := newTestTermsClaim(t, srv)
	ctx := context.Background()

	if ok, _ := c.Claim(ctx, "v3"); !ok {
		t.Fatal("v3 must be claimable")
	}
	if ok, _ := c.Claim(ctx, "v4"); !ok {
		t.Fatal("a different version must have its own claim")
	}
}

// 印に寿命を付けない。付けると、切れた後の起動で同じ版をもう一度配ってしまう。
func TestTermsBroadcastClaim_MarkDoesNotExpire(t *testing.T) {
	srv := miniredis.RunT(t)
	c := newTestTermsClaim(t, srv)

	if _, err := c.Claim(context.Background(), "v5"); err != nil {
		t.Fatalf("Claim returned an error: %v", err)
	}
	if ttl := srv.TTL("test:" + termsBroadcastKeyPrefix + "v5"); ttl != 0 {
		t.Fatalf("TTL = %v, want no expiry on the claim mark", ttl)
	}
}
