package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRateLimiter は時計を止めた置き場を返す。進めるのは advance で。
func newTestRateLimiter(t *testing.T, srv *miniredis.Miniredis, rate, burst float64) (*RateLimiterStore, func(time.Duration)) {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	s := NewRateLimiterStore(client, "test:", rate, burst)
	clock := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return clock }
	return s, func(d time.Duration) { clock = clock.Add(d) }
}

func allowN(t *testing.T, s *RateLimiterStore, id string, n int) int {
	t.Helper()
	allowed := 0
	for range n {
		ok, err := s.Allow(id)
		if err != nil {
			t.Fatalf("Allow returned an error: %v", err)
		}
		if ok {
			allowed++
		}
	}
	return allowed
}

func TestRateLimiter_AllowsUpToBurstThenDenies(t *testing.T) {
	srv := miniredis.RunT(t)
	s, _ := newTestRateLimiter(t, srv, 20, 40)

	if got := allowN(t, s, "1.2.3.4", 50); got != 40 {
		t.Fatalf("allowed %d of 50, want the burst of 40", got)
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	srv := miniredis.RunT(t)
	s, advance := newTestRateLimiter(t, srv, 20, 40)

	allowN(t, s, "1.2.3.4", 40)
	if ok, _ := s.Allow("1.2.3.4"); ok {
		t.Fatal("the bucket must be empty right after the burst")
	}

	// 20/秒なので、0.5秒で10本ぶん戻る。
	advance(500 * time.Millisecond)
	if got := allowN(t, s, "1.2.3.4", 20); got != 10 {
		t.Fatalf("allowed %d after half a second, want 10 refilled", got)
	}
}

func TestRateLimiter_CountsEachIdentifierSeparately(t *testing.T) {
	srv := miniredis.RunT(t)
	s, _ := newTestRateLimiter(t, srv, 20, 40)

	allowN(t, s, "1.2.3.4", 40)
	if ok, _ := s.Allow("5.6.7.8"); !ok {
		t.Fatal("a different IP must have its own bucket")
	}
}

// この変更の要。台を2つ立てても、1つのIPに許す量は合わせて burst のまま。
// 以前のメモリ実装では台ごとに別のバケツを持つので、ここが 80 になっていた。
func TestRateLimiter_SharesOneBucketAcrossInstances(t *testing.T) {
	srv := miniredis.RunT(t)
	s1, _ := newTestRateLimiter(t, srv, 20, 40)
	s2, _ := newTestRateLimiter(t, srv, 20, 40)

	total := allowN(t, s1, "1.2.3.4", 30) + allowN(t, s2, "1.2.3.4", 30)
	if total != 40 {
		t.Fatalf("allowed %d across two instances, want the shared burst of 40", total)
	}
}

// 時計が巻き戻った台から叩かれても、トークンは増えない。
func TestRateLimiter_DoesNotRefillOnABackwardsClock(t *testing.T) {
	srv := miniredis.RunT(t)
	s, advance := newTestRateLimiter(t, srv, 20, 40)

	allowN(t, s, "1.2.3.4", 40)
	advance(-10 * time.Second)

	if ok, _ := s.Allow("1.2.3.4"); ok {
		t.Fatal("a backwards clock must not hand out tokens")
	}
}

// Redis が応答しないときは通す。速度制限を理由に本体を止めない。
func TestRateLimiter_LetsTrafficThroughWhenRedisIsDown(t *testing.T) {
	srv := miniredis.RunT(t)
	s, _ := newTestRateLimiter(t, srv, 20, 40)
	srv.Close()

	ok, err := s.Allow("1.2.3.4")
	if err != nil {
		t.Fatalf("a Redis outage must not surface as an error: %v", err)
	}
	if !ok {
		t.Fatal("a Redis outage must not deny the request")
	}
}
