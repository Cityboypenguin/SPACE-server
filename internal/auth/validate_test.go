package auth

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type testRevokedTokens struct {
	repository.RevokedTokenRepository
}

func (*testRevokedTokens) IsRevoked(context.Context, string) (bool, error) { return false, nil }

type testUsers struct {
	repository.UserRepository
	user    *model.User
	version int64
}

func (r *testUsers) GetUserByID(context.Context, int64) (*model.User, error) { return r.user, nil }
func (r *testUsers) GetCredentialsVersionByID(context.Context, int64) (int64, error) {
	return r.version, nil
}

type testAdministrators struct {
	repository.AdministratorRepository
	admin *model.Administrator
}

func (r *testAdministrators) GetAdministratorByID(context.Context, int64) (*model.Administrator, error) {
	return r.admin, nil
}

func TestAccessTokenRejectsDeletedAdministrator(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	token, err := GenerateAccessToken(7, "administrator")
	if err != nil {
		t.Fatal(err)
	}
	admins := &testAdministrators{admin: &model.Administrator{ID: 7}}
	if _, err := ValidateAndVerifyToken(context.Background(), token, &testRevokedTokens{}, &testUsers{}, admins); err != nil {
		t.Fatalf("existing administrator rejected: %v", err)
	}
	admins.admin = nil
	if _, err := ValidateAndVerifyToken(context.Background(), token, &testRevokedTokens{}, &testUsers{}, admins); err == nil {
		t.Fatal("deleted administrator's access token remained valid")
	}
}

func TestAccessTokenRejectsPreviousCredentialsVersion(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	users := &testUsers{user: &model.User{ID: 7, Status: model.UserStatusActive}, version: 0}
	token, err := GenerateUserAccessToken(7, "user", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAndVerifyToken(context.Background(), token, &testRevokedTokens{}, users, &testAdministrators{}); err != nil {
		t.Fatalf("current token rejected: %v", err)
	}
	users.version = 1
	if _, err := ValidateAndVerifyToken(context.Background(), token, &testRevokedTokens{}, users, &testAdministrators{}); err == nil {
		t.Fatal("old token remained valid after credentials changed")
	}
	legacyToken, err := GenerateAccessToken(7, "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateAndVerifyToken(context.Background(), legacyToken, &testRevokedTokens{}, users, &testAdministrators{}); err == nil {
		t.Fatal("legacy token without a credentials version remained valid")
	}
}

func TestTokensHaveUniqueJWTIDs(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	seen := make(map[string]bool)
	for i := 0; i < 32; i++ {
		token, err := GenerateUserAccessToken(7, "user", 0)
		if err != nil {
			t.Fatal(err)
		}
		claims, err := ValidateAccessToken(token)
		if err != nil {
			t.Fatal(err)
		}
		if claims.RegisteredClaims.ID == "" || seen[claims.RegisteredClaims.ID] {
			t.Fatal("JWT ID is empty or repeated")
		}
		seen[claims.RegisteredClaims.ID] = true
	}
}
