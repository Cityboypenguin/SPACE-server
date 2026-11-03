package model

import (
	"strings"
	"testing"
)

func TestStampIDSyntax(t *testing.T) {
	for _, id := range []string{"stamp:thanks", "stamp:new-custom-id"} {
		if !IsValidMessageStampID(id) {
			t.Errorf("valid syntax: %s", id)
		}
	}
	for _, id := range []string{"", "stamp:", "stamp:UPPER", "stamp:a/b", "👍", strings.Repeat("a", 65)} {
		if IsValidMessageStampID(id) {
			t.Errorf("invalid syntax: %s", id)
		}
	}
}
