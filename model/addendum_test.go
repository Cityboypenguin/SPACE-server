package model

import (
	"strings"
	"testing"
	"time"
)

func TestNewAddendum(t *testing.T) {
	now := time.Unix(1700000000, 0)
	a, err := NewAddendum(1, 2, "  補足です  ", now)
	if err != nil || a.Body != "補足です" || a.TargetID != 1 || a.UserID != 2 {
		t.Fatalf("a = %+v, err = %v", a, err)
	}
	if _, err := NewAddendum(1, 2, "   ", now); err == nil {
		t.Error("an empty addendum must be rejected")
	}
	if _, err := NewAddendum(1, 2, strings.Repeat("あ", MaxAddendumChars), now); err != nil {
		t.Errorf("exactly the limit must pass: %v", err)
	}
	if _, err := NewAddendum(1, 2, strings.Repeat("あ", MaxAddendumChars+1), now); err == nil {
		t.Error("over the limit must be rejected")
	}
}
