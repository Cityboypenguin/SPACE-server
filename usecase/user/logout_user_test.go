package user

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
)

type logoutTokens struct {
	consumingTokens
	revoked map[string]bool
}

func (r *logoutTokens) RevokeToken(_ context.Context, token string, _ int64) error {
	r.revoked[token] = true
	return nil
}

// ログアウトしたあとは、リフレッシュトークンでもログインし直せない。
func TestLogoutRevokesRefreshToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	access, err := auth.GenerateUserAccessToken(7, "user", 2)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := auth.GenerateUserRefreshToken(7, "user", 2)
	if err != nil {
		t.Fatal(err)
	}
	tokens := &logoutTokens{consumingTokens: consumingTokens{consumed: map[string]bool{}}, revoked: map[string]bool{}}

	if err := NewLogoutUserUseCase(tokens).Execute(context.Background(), access, &refresh); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if !tokens.revoked[access] {
		t.Fatal("access token was not revoked")
	}
	if _, err := NewRefreshUserTokenUseCase(&refreshUserRepo{}, tokens).Execute(context.Background(), refresh); err == nil {
		t.Fatal("refresh token still works after logout")
	}
}

// アクセストークンが期限切れでログアウトが失敗しても、リフレッシュトークンは止める。
func TestLogoutRevokesRefreshTokenEvenWhenAccessTokenIsInvalid(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	refresh, err := auth.GenerateUserRefreshToken(7, "user", 2)
	if err != nil {
		t.Fatal(err)
	}
	tokens := &logoutTokens{consumingTokens: consumingTokens{consumed: map[string]bool{}}, revoked: map[string]bool{}}

	if err := NewLogoutUserUseCase(tokens).Execute(context.Background(), "expired", &refresh); err == nil {
		t.Fatal("logout with an invalid access token should fail")
	}
	if !tokens.consumed[refresh] {
		t.Fatal("refresh token was not revoked")
	}
}
