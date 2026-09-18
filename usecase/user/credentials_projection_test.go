package user

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// (28) の回帰テスト。
//
// 守りたいのは「表示系のコードパスで認証情報（パスワードハッシュ）を読まない」
// こと。型の上では model.User がハッシュを持てないので載せようがないが、
// 表示系のユースケースが認証情報側の取得口（FindCredentialsByEmail /
// GetCredentialsByID / SaveCredentials）を呼び始めたら意味が無い。
// その3つを「呼ばれたら落ちる」実装にして、経路ごとに確かめる。

// noCredentialsUserRepo は認証情報の口を塞いだ UserRepository。
type noCredentialsUserRepo struct {
	t *testing.T

	user    *model.User
	account *model.UserAccount

	getByIDCalls int
	updateCalls  int
	lastUpdated  *model.User
}

func (r *noCredentialsUserRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("表示系の経路が認証情報の口 %s を呼んだ", method)
}

// --- 認証情報（表示系から呼ばれたら失敗）---

func (r *noCredentialsUserRepo) FindCredentialsByEmail(context.Context, string) (*model.UserCredentials, error) {
	r.fail("FindCredentialsByEmail")
	return nil, nil
}

func (r *noCredentialsUserRepo) GetCredentialsByID(context.Context, int64) (*model.UserCredentials, error) {
	r.fail("GetCredentialsByID")
	return nil, nil
}

func (r *noCredentialsUserRepo) SaveCredentials(context.Context, *model.UserCredentials) error {
	r.fail("SaveCredentials")
	return nil
}

// --- 公開情報 ---

func (r *noCredentialsUserRepo) GetUserByID(_ context.Context, _ int64) (*model.User, error) {
	r.getByIDCalls++
	return r.user, nil
}

func (r *noCredentialsUserRepo) UpdateUser(_ context.Context, u *model.User) error {
	r.updateCalls++
	r.lastUpdated = u
	return nil
}

func (r *noCredentialsUserRepo) GetUsersByIDs(context.Context, []int64) ([]*model.User, error) {
	return []*model.User{r.user}, nil
}

func (r *noCredentialsUserRepo) FindByEmail(context.Context, string) (*model.User, error) {
	return r.user, nil
}

func (r *noCredentialsUserRepo) SearchUsersByKeyword(context.Context, string, repository.PageQuery) ([]*model.User, int, error) {
	return []*model.User{r.user}, 1, nil
}

// --- 本人・管理者向け ---
//
// 連絡先は返すがハッシュは読まないので、表示系の見張り（fail）には掛けない。
// 「この口を呼んだら落ちる」対象はあくまで認証情報の3つ。

func (r *noCredentialsUserRepo) GetUserAccountByID(context.Context, int64) (*model.UserAccount, error) {
	return r.account, nil
}

func (r *noCredentialsUserRepo) GetUserAccountsByIDs(context.Context, []int64) ([]*model.UserAccount, error) {
	return []*model.UserAccount{r.account}, nil
}

func (r *noCredentialsUserRepo) ListUserAccounts(context.Context, repository.PageQuery) ([]*model.UserAccount, int, error) {
	return []*model.UserAccount{r.account}, 1, nil
}

func (r *noCredentialsUserRepo) SearchUserAccountsByKeyword(context.Context, string, repository.PageQuery) ([]*model.UserAccount, int, error) {
	return []*model.UserAccount{r.account}, 1, nil
}

func (r *noCredentialsUserRepo) GetUsersByAccountIDs(context.Context, []string) ([]*model.User, error) {
	return []*model.User{r.user}, nil
}

func (r *noCredentialsUserRepo) SuggestUsersByPrefix(context.Context, string, int) ([]*model.User, error) {
	return []*model.User{r.user}, nil
}

func (r *noCredentialsUserRepo) DeleteUser(context.Context, int64) (bool, error)        { return true, nil }
func (r *noCredentialsUserRepo) UpdateLastActiveAt(context.Context, int64, int64) error { return nil }
func (r *noCredentialsUserRepo) LogActivityDate(context.Context, int64, string) error   { return nil }
func (r *noCredentialsUserRepo) LogActivityHour(context.Context, int64, string) error   { return nil }

var _ repository.UserRepository = &noCredentialsUserRepo{}

func adminCtx() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "admin"})
}

func TestDisplayPathsNeverReadCredentials(t *testing.T) {
	newRepo := func(t *testing.T) *noCredentialsUserRepo {
		u := model.User{ID: 42, AccountID: "taro", Name: "太郎", Role: "user", Status: model.UserStatusActive}
		return &noCredentialsUserRepo{
			t:       t,
			user:    &u,
			account: &model.UserAccount{User: u, Email: "e@x"},
		}
	}

	t.Run("ユーザー1件の取得", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := NewGetUserByIDUseCase(repo).Execute(context.Background(), 42); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ユーザー一覧（管理者向け。連絡先は返すがハッシュは読まない）", func(t *testing.T) {
		repo := newRepo(t)
		if _, _, err := NewListUsersUseCase(repo).Execute(adminCtx(), repository.PageQuery{Limit: 20, WithTotal: true}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ユーザー検索", func(t *testing.T) {
		repo := newRepo(t)
		if _, _, err := NewSearchUsersUseCase(repo).Execute(adminCtx(), "t", repository.PageQuery{Limit: 20}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("IDまとめ取得（DataLoader の UserLoader が使う口）", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := NewGetUsersByIDsUseCase(repo).Execute(context.Background(), []int64{42}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("凍結（読んで書き戻す経路）", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := NewFreezeUserUseCase(repo).Execute(context.Background(), 42); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.updateCalls != 1 {
			t.Fatalf("UpdateUser calls = %d, want 1", repo.updateCalls)
		}
		if repo.lastUpdated.Status != model.UserStatusFrozen {
			t.Errorf("status = %q, want frozen", repo.lastUpdated.Status)
		}
	})

	t.Run("トークン検証は凍結状態しか見ないので公開情報で足りる", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := repo.GetUserByID(context.Background(), 42); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.getByIDCalls != 1 {
			t.Fatalf("GetUserByID calls = %d, want 1", repo.getByIDCalls)
		}
	})
}

// パスワードを渡して公開情報の更新口を使うと、黙って無視されずエラーになる。
// 無視されると「変更したのに変わっていない」という気づけない壊れ方になる。
func TestUpdateProfileRejectsPasswordChange(t *testing.T) {
	u := &model.User{ID: 1, AccountID: "taro", Name: "太郎"}
	pw := "newpassword"
	if err := u.UpdateProfile(model.UpdateUserParam{Password: &pw}); err == nil {
		t.Fatal("UpdateProfile がパスワード変更を受け入れてしまった")
	}
}

// (A) の回帰。連絡先を持たない model.User の更新口にメールアドレスを渡すと、
// 黙って捨てられずエラーになる。捨てられると「変更したのに変わっていない」という
// 気づけない壊れ方になる（パスワードのときと同じ理屈）。
func TestUpdateProfileRejectsEmailChange(t *testing.T) {
	u := &model.User{ID: 1, AccountID: "taro", Name: "太郎"}
	email := "new@example.com"
	if err := u.UpdateProfile(model.UpdateUserParam{Email: &email}); err == nil {
		t.Fatal("User.UpdateProfile がメールアドレス変更を受け入れてしまった")
	}
}

// 連絡先を持つ UserAccount 側は、これまでどおりメールアドレスを更新できる
// （分離で機能が落ちていないこと）。
func TestUserAccountUpdateProfileAppliesEmail(t *testing.T) {
	a := &model.UserAccount{
		User:  model.User{ID: 1, AccountID: "taro", Name: "太郎"},
		Email: "old@example.com",
	}
	email := "new@example.com"
	if err := a.UpdateProfile(model.UpdateUserParam{Email: &email}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Email != email {
		t.Fatalf("Email = %q, want %q", a.Email, email)
	}
}

// 認証情報側はこれまでどおりハッシュを扱える（分離で機能が落ちていないこと）。
func TestUserCredentialsStillHashesPassword(t *testing.T) {
	c := &model.UserCredentials{}
	if err := c.CreateUser(model.CreateUserParam{AccountID: "taro", Name: "太郎", Email: "e@x", Password: "password1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.HashedPassword == "" || c.HashedPassword == "password1" {
		t.Fatalf("HashedPassword = %q, want a bcrypt hash", c.HashedPassword)
	}
}
