package apperr

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodeOf_ReturnsAppErrCode(t *testing.T) {
	err := Forbidden("nope")
	if got := CodeOf(err); got != CodeForbidden {
		t.Errorf("CodeOf = %q, want %q", got, CodeForbidden)
	}
}

func TestCodeOf_UnwrapsWrappedAppErr(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", NotFound("missing"))
	if got := CodeOf(wrapped); got != CodeNotFound {
		t.Errorf("CodeOf = %q, want %q", got, CodeNotFound)
	}
}

func TestCodeOf_DefaultsToInternal(t *testing.T) {
	if got := CodeOf(errors.New("plain error")); got != CodeInternal {
		t.Errorf("CodeOf = %q, want %q", got, CodeInternal)
	}
}

func TestError_UnwrapPreservesCause(t *testing.T) {
	cause := errors.New("root cause")
	err := Wrap(CodeConflict, "conflict", cause)
	if !errors.Is(err, cause) {
		t.Error("Wrap should preserve the underlying cause for errors.Is")
	}
}
