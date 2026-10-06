package accountpurge

import (
	"testing"
	"time"
)

func TestNextRunDelay(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 6, h, m, 0, 0, jst) }
	for _, tt := range []struct {
		name   string
		now    time.Time
		failed bool
		want   time.Duration
	}{
		{"before 4am runs the same day", at(1, 30), false, 2*time.Hour + 30*time.Minute},
		{"exactly 4am waits a full day", at(4, 0), false, 24 * time.Hour},
		{"after 4am runs the next day", at(10, 0), false, 18 * time.Hour},
		{"a failure retries within the hour", at(10, 0), true, time.Hour},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextRunDelay(tt.now, tt.failed); got != tt.want {
				t.Fatalf("nextRunDelay = %v, want %v", got, tt.want)
			}
		})
	}
}
