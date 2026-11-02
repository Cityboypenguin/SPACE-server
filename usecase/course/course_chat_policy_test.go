package course

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type courseChatSetting struct {
	repository.SystemSettingRepository
	value string
}

func (s courseChatSetting) Get(context.Context, string) (string, error) { return s.value, nil }

func TestEnsureCourseChatEnabled(t *testing.T) {
	user := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "user"})
	admin := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "administrator"})

	if err := EnsureCourseChatEnabled(user, courseChatSetting{value: "true"}); err != nil {
		t.Errorf("enabled: %v", err)
	}
	// 値が無い（マイグレーション前）なら止めない。
	if err := EnsureCourseChatEnabled(user, courseChatSetting{value: ""}); err != nil {
		t.Errorf("unset: %v", err)
	}
	if err := EnsureCourseChatEnabled(user, courseChatSetting{value: "false"}); apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Errorf("stopped: got %v, want forbidden", err)
	}
	// 通報の対応があるので、管理者は止めない。
	if err := EnsureCourseChatEnabled(admin, courseChatSetting{value: "false"}); err != nil {
		t.Errorf("admin while stopped: %v", err)
	}
}
