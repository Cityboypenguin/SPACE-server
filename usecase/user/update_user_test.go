package user

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// 書き込みの口を持たないリポジトリ。断るべき入力で、DB まで行かないことを確かめる。
type untouchedProfileRepo struct{ userProfileWriteRepository }

func TestUpdateUserRejectsSelfEmailChange(t *testing.T) {
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 7, Role: "user"})
	email := "someone@example.com"
	current := "currentPass1"
	_, err := NewUpdateUserUseCase(untouchedProfileRepo{}).Execute(ctx, 7, model.UpdateUserParam{Email: &email}, &current, true)
	if err == nil {
		t.Fatal("本人がメールアドレスを変えられた")
	}
}

func TestUpdateUserRejectsShortPassword(t *testing.T) {
	short := "1234567"
	current := "currentPass1"
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		require bool
	}{
		{"本人", auth.WithClaims(context.Background(), &auth.Claims{ID: 7, Role: "user"}), true},
		{"管理者", auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "administrator"}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewUpdateUserUseCase(untouchedProfileRepo{}).Execute(tc.ctx, 7, model.UpdateUserParam{Password: &short}, &current, tc.require)
			if err == nil {
				t.Fatal("8文字未満のパスワードが通った")
			}
		})
	}
}
