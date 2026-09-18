package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// fakeBlockerRepo はブロック一覧の取得だけを差し替える。埋め込んだ interface は
// nil のままなので、ミドルウェアがそれ以外を呼べば panic する（呼ばないことも検査）。
type fakeBlockerRepo struct {
	repository.BlockerRepository

	ids []int64
	err error
}

func (f *fakeBlockerRepo) GetBlockedAndBlockerIDs(context.Context, int64) ([]int64, error) {
	return f.ids, f.err
}

// runBlockFilter は BlockFilter を1リクエスト通し、ハンドラまで届いた Context を返す。
func runBlockFilter(t *testing.T, repo repository.BlockerRepository, claims *auth.Claims) context.Context {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	if claims != nil {
		req = req.WithContext(auth.WithClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	var got context.Context
	handler := BlockFilter(repo)(func(c echo.Context) error {
		got = c.Request().Context()
		return nil
	})
	if err := handler(c); err != nil {
		t.Fatalf("BlockFilter returned an unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("the next handler was not called")
	}
	return got
}

// ブロック一覧が読めなかったときにフェイルオープンしないこと。
//
// 以前はエラーを握り潰して Context に何も入れず、後段からは「ブロック相手が
// 居ない」と全く同じに見えていた。その結果、DB 障害中は本来見えないはずの投稿・
// ユーザーが素通しで出ていた。
func TestBlockFilter_DoesNotFailOpenWhenTheBlockListCannotBeLoaded(t *testing.T) {
	repo := &fakeBlockerRepo{err: errors.New("database is down")}

	ctx := runBlockFilter(t, repo, &auth.Claims{ID: 42})

	ids, err := BlockListFromContext(ctx)
	if err == nil {
		t.Fatal("expected BlockListFromContext to report that the list is unavailable")
	}
	if !errors.Is(err, ErrBlockListUnavailable) {
		t.Fatalf("expected ErrBlockListUnavailable, got %v", err)
	}
	if ids != nil {
		t.Fatalf("expected no ids alongside the failure, got %v", ids)
	}
}

// 正常時は従来どおり一覧が渡ること。
func TestBlockFilter_PassesTheLoadedBlockList(t *testing.T) {
	repo := &fakeBlockerRepo{ids: []int64{7, 9}}

	ctx := runBlockFilter(t, repo, &auth.Claims{ID: 42})

	ids, err := BlockListFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != 7 || ids[1] != 9 {
		t.Fatalf("expected the loaded ids to reach the handler, got %v", ids)
	}
}

// 「ブロック相手が1人も居ない」は失敗ではないこと（読めているので除外は不要）。
func TestBlockFilter_EmptyBlockListIsNotAFailure(t *testing.T) {
	repo := &fakeBlockerRepo{ids: nil}

	ctx := runBlockFilter(t, repo, &auth.Claims{ID: 42})

	ids, err := BlockListFromContext(ctx)
	if err != nil {
		t.Fatalf("an empty block list must not be reported as a failure, got %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids, got %v", ids)
	}
}

// 未認証リクエストでは除外そのものが不要なので、失敗扱いにしないこと。
func TestBlockFilter_UnauthenticatedRequestHasNoBlockList(t *testing.T) {
	repo := &fakeBlockerRepo{err: errors.New("must not be called")}

	ctx := runBlockFilter(t, repo, nil)

	ids, err := BlockListFromContext(ctx)
	if err != nil {
		t.Fatalf("an unauthenticated request must not be reported as a failure, got %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids, got %v", ids)
	}
}
