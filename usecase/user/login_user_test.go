package user

import (
	"context"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// loginRepo はログインに要る口だけを持つ代役。
type loginRepo struct {
	repository.UserRepository
	creds       *model.UserCredentials
	reactivated int
	// reactivateOK が false なら、取り消しの時点でもう退会手続き中ではなかった
	// （猶予が切れて個人情報を消された直後など）ことにする。
	reactivateOK bool
}

func (r *loginRepo) FindCredentialsByEmail(context.Context, string) (*model.UserCredentials, error) {
	return r.creds, nil
}

func (r *loginRepo) ReactivateUser(context.Context, int64) (bool, error) {
	r.reactivated++
	return r.reactivateOK, nil
}

func (r *loginRepo) DeactivateUser(context.Context, int64, time.Time) (bool, error) {
	return false, nil
}

func newLoginRepo(t *testing.T, status string) *loginRepo {
	t.Helper()
	var c model.UserCredentials
	if err := c.CreateUser(model.CreateUserParam{AccountID: "taro", Name: "太郎", Email: "taro@example.com", Password: "correct-password"}); err != nil {
		t.Fatal(err)
	}
	c.ID = 1
	c.Role = "user"
	c.Status = status
	return &loginRepo{creds: &c, reactivateOK: true}
}

func TestLoginRestoresDeactivatedAccount(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	repo := newLoginRepo(t, model.UserStatusDeactivated)

	result, err := NewLoginUserUseCase(repo).Execute(context.Background(), "taro@example.com", "correct-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !result.AccountRestored || repo.reactivated != 1 {
		t.Fatalf("restored=%v reactivated=%d, want the withdrawal cancelled", result.AccountRestored, repo.reactivated)
	}
	if result.User.Status != model.UserStatusActive {
		t.Fatalf("status = %q, want active", result.User.Status)
	}
}

// パスワードが違えば、本人と分からないので取り消さない。
func TestLoginWithWrongPasswordDoesNotRestore(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	repo := newLoginRepo(t, model.UserStatusDeactivated)

	if _, err := NewLoginUserUseCase(repo).Execute(context.Background(), "taro@example.com", "wrong-password"); err == nil {
		t.Fatal("want an error for the wrong password")
	}
	if repo.reactivated != 0 {
		t.Fatal("a wrong password cancelled the withdrawal")
	}
}

// 取り消そうとした時にはもう消されていた（猶予切れの処理と重なった）なら、ログインさせない。
func TestLoginFailsWhenAccountWasPurgedMeanwhile(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	repo := newLoginRepo(t, model.UserStatusDeactivated)
	repo.reactivateOK = false

	if _, err := NewLoginUserUseCase(repo).Execute(context.Background(), "taro@example.com", "correct-password"); err == nil {
		t.Fatal("want login to fail for an account that is gone")
	}
}

func TestLoginOfActiveAccountIsNotARestore(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-signing-secret")
	repo := newLoginRepo(t, model.UserStatusActive)

	result, err := NewLoginUserUseCase(repo).Execute(context.Background(), "taro@example.com", "correct-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.AccountRestored || repo.reactivated != 0 {
		t.Fatalf("restored=%v reactivated=%d for an active account", result.AccountRestored, repo.reactivated)
	}
}
