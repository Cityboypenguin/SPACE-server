package administrator

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 件数だけが要る場面で行を取って捨てないこと（以前は ListAdministrators(ctx, 1, 0) の
// 戻り値の total だけを使っていた）を、fake 側で ListAdministrators を呼ばれたら
// 落とすことで担保する。
type fakeAdminRepo struct {
	repository.AdministratorRepository
	t        *testing.T
	count    int
	countErr error
	deleted  bool
	// countedForUpdate は「削除の判定がロックを取る版の COUNT を使ったか」。
	// ロックなしで数えると同時削除で管理者が0人になりうるので、ここを見張る。
	countedForUpdate bool
}

func (f *fakeAdminRepo) CountAdministrators(_ context.Context) (int, error) {
	return f.count, f.countErr
}

func (f *fakeAdminRepo) CountAdministratorsForUpdate(_ context.Context) (int, error) {
	f.countedForUpdate = true
	return f.count, f.countErr
}

func (f *fakeAdminRepo) ListAdministrators(_ context.Context, _ repository.PageQuery) ([]*model.Administrator, int, error) {
	f.t.Fatal("ListAdministrators must not be called just to get a count")
	return nil, 0, nil
}

func (f *fakeAdminRepo) DeleteAdministrator(_ context.Context, _ int64) (bool, error) {
	f.deleted = true
	return true, nil
}

// inlineTxManager はトランザクションの境界だけを再現する（実 DB を使わない）。
// ロックが本当に効くかは MySQL でしか確かめられないので、それは
// infra/mysql 側の並行テストに任せ、ここでは「1つのトランザクションに入っているか」
// と「ロックを取る版で数えているか」だけを見る。
type inlineTxManager struct{ calls int }

func administratorContext() context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "administrator"})
}

func (m *inlineTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.calls++
	return fn(ctx)
}

func TestCountAdministrators_UsesCountQuery(t *testing.T) {
	repo := &fakeAdminRepo{t: t, count: 3}
	got, err := NewCountAdministratorsUseCase(repo).Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
}

func TestCountAdministrators_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &fakeAdminRepo{t: t, countErr: boom}
	if _, err := NewCountAdministratorsUseCase(repo).Execute(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
}

func TestDeleteAdministrator_KeepsTheLastOne(t *testing.T) {
	repo := &fakeAdminRepo{t: t, count: 1}
	ok, err := NewDeleteAdministratorUseCase(repo, &inlineTxManager{}).Execute(administratorContext(), 1)
	if err == nil {
		t.Fatal("expected an error when deleting the last administrator")
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
	if repo.deleted {
		t.Fatal("the last administrator must not be deleted")
	}
}

func TestDeleteAdministrator_DeletesWhenOthersRemain(t *testing.T) {
	repo := &fakeAdminRepo{t: t, count: 2}
	tx := &inlineTxManager{}
	ok, err := NewDeleteAdministratorUseCase(repo, tx).Execute(administratorContext(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || !repo.deleted {
		t.Fatalf("ok = %v, deleted = %v; want the delete to go through", ok, repo.deleted)
	}
	if tx.calls != 1 {
		t.Fatalf("RunInTx calls = %d, want the count and the delete to share one transaction", tx.calls)
	}
	if !repo.countedForUpdate {
		t.Fatal("the guard must count with a row lock, otherwise concurrent deletes can remove every administrator")
	}
}
