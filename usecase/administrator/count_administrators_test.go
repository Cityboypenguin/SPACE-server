package administrator

import (
	"context"
	"errors"
	"testing"

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
}

func (f *fakeAdminRepo) CountAdministrators(_ context.Context) (int, error) {
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
	ok, err := NewDeleteAdministratorUseCase(repo).Execute(context.Background(), 1)
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
	ok, err := NewDeleteAdministratorUseCase(repo).Execute(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || !repo.deleted {
		t.Fatalf("ok = %v, deleted = %v; want the delete to go through", ok, repo.deleted)
	}
}
