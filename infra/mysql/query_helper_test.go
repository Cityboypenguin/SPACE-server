package mysql

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/middleware"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/labstack/echo/v4"
)

// blockRepoStub はブロック一覧の取得だけを差し替える。
type blockRepoStub struct {
	repository.BlockerRepository

	ids []int64
	err error
}

func (s *blockRepoStub) GetBlockedAndBlockerIDs(context.Context, int64) ([]int64, error) {
	return s.ids, s.err
}

// requestContextWithBlockList は BlockFilter ミドルウェアを実際に1本通した
// Context を返す。ここを手で組むと「ミドルウェアが何を詰めるか」を二重に
// 定義することになり、片方だけ直る事故につながるため通している。
func requestContextWithBlockList(t *testing.T, repo repository.BlockerRepository) context.Context {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{ID: 42}))
	c := echo.New().NewContext(req, httptest.NewRecorder())

	var got context.Context
	handler := middleware.BlockFilter(repo)(func(c echo.Context) error {
		got = c.Request().Context()
		return nil
	})
	if err := handler(c); err != nil {
		t.Fatalf("BlockFilter returned an unexpected error: %v", err)
	}
	return got
}

// ブロック一覧が読めていないリクエストでは、除外なしのクエリを組み立てず
// エラーを返すこと。
//
// 以前はこの関数が必ず成功する形だったので、一覧の取得に失敗したリクエストでは
// NOT IN が付かないクエリがそのまま実行され、ブロックしたはずの相手の投稿が
// 見えていた。
func TestAppendBlockFilter_FailsClosedWhenTheBlockListIsUnavailable(t *testing.T) {
	ctx := requestContextWithBlockList(t, &blockRepoStub{err: errors.New("database is down")})

	base := "SELECT id FROM posts WHERE deleted_at IS NULL"
	query, args, err := AppendBlockFilter(ctx, base, []interface{}{}, "user_id")
	if err == nil {
		t.Fatal("expected AppendBlockFilter to fail when the block list is unavailable")
	}
	if !errors.Is(err, middleware.ErrBlockListUnavailable) {
		t.Fatalf("expected ErrBlockListUnavailable, got %v", err)
	}
	// 呼び出し側が戻り値をうっかり使っても素通しのクエリにならないこと。
	if query != "" || args != nil {
		t.Fatalf("expected no usable query on failure, got %q / %v", query, args)
	}
}

// 一覧が読めていれば従来どおり NOT IN が付くこと。
func TestAppendBlockFilter_ExcludesTheBlockedIDs(t *testing.T) {
	ctx := requestContextWithBlockList(t, &blockRepoStub{ids: []int64{7, 9}})

	base := "SELECT id FROM posts WHERE deleted_at IS NULL"
	query, args, err := AppendBlockFilter(ctx, base, []interface{}{}, "user_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(query, "AND user_id NOT IN (?,?)") {
		t.Fatalf("expected the NOT IN clause, got %q", query)
	}
	if len(args) != 2 || args[0] != int64(7) || args[1] != int64(9) {
		t.Fatalf("expected the blocked ids as args, got %v", args)
	}
}

// ブロック相手が居ないリクエストでは条件を足さないこと（従来どおり）。
func TestAppendBlockFilter_LeavesTheQueryAloneWithoutBlockedUsers(t *testing.T) {
	ctx := requestContextWithBlockList(t, &blockRepoStub{ids: nil})

	base := "SELECT id FROM posts WHERE deleted_at IS NULL"
	query, args, err := AppendBlockFilter(ctx, base, []interface{}{}, "user_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query != base {
		t.Fatalf("expected the query to be unchanged, got %q", query)
	}
	if len(args) != 0 {
		t.Fatalf("expected no extra args, got %v", args)
	}
}

// ---------------------------------------------------------------------------
// inChunks — プレースホルダ上限での分割
// ---------------------------------------------------------------------------

// 上限に届かない件数では分割せず exec を1回だけ呼ぶこと。
// ここが割れると、投票の選択肢や添付のように数十件で収まる箇所まで
// クエリ本数が増えてしまう。
func TestInChunks_DoesNotSplitBelowTheLimit(t *testing.T) {
	rows := make([]int64, 100)
	calls := 0
	if err := inChunks(rows, 1, func(chunk []int64) error {
		calls++
		if len(chunk) != len(rows) {
			t.Errorf("chunk size = %d, want %d", len(chunk), len(rows))
		}
		return nil
	}); err != nil {
		t.Fatalf("inChunks returned an unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("exec calls = %d, want 1", calls)
	}
}

// 上限を超える件数は分割され、かつ1件も落とさず・順序も保つこと。
// 分割で取りこぼすと「件数が多いときだけ結果が欠ける」という、
// 応答を見ても気づけない壊れ方になる。
func TestInChunks_SplitsAtTheParameterLimit(t *testing.T) {
	const cols = 1
	size := bulkInsertChunkSize(cols)

	rows := make([]int64, size*2+7)
	for i := range rows {
		rows[i] = int64(i)
	}

	var seen []int64
	calls := 0
	if err := inChunks(rows, cols, func(chunk []int64) error {
		calls++
		if len(chunk) > size {
			t.Errorf("chunk size = %d, exceeds the limit %d", len(chunk), size)
		}
		seen = append(seen, chunk...)
		return nil
	}); err != nil {
		t.Fatalf("inChunks returned an unexpected error: %v", err)
	}

	if calls != 3 {
		t.Errorf("exec calls = %d, want 3", calls)
	}
	if len(seen) != len(rows) {
		t.Fatalf("saw %d rows, want %d", len(seen), len(rows))
	}
	for i, v := range seen {
		if v != rows[i] {
			t.Fatalf("row %d = %d, want %d (order must be preserved)", i, v, rows[i])
		}
	}
}

// 列数が多いほど1回に載せる行数は減ること。列数を無視して行数で切ると、
// 列の多いテーブルでプレースホルダ上限を超える。
func TestBulkInsertChunkSize_ShrinksWithMoreColumns(t *testing.T) {
	if got, want := bulkInsertChunkSize(1), maxBulkInsertParams; got != want {
		t.Errorf("bulkInsertChunkSize(1) = %d, want %d", got, want)
	}
	if got, want := bulkInsertChunkSize(6), maxBulkInsertParams/6; got != want {
		t.Errorf("bulkInsertChunkSize(6) = %d, want %d", got, want)
	}
	// 1行だけで上限を超えるような列数でも、0 を返して無限ループにしないこと。
	if got := bulkInsertChunkSize(maxBulkInsertParams + 1); got != 1 {
		t.Errorf("bulkInsertChunkSize(huge) = %d, want 1", got)
	}
	if got := bulkInsertChunkSize(0); got != 1 {
		t.Errorf("bulkInsertChunkSize(0) = %d, want 1", got)
	}
}

// 空スライスでは exec を一度も呼ばないこと。IN () は MySQL の構文エラーなので、
// 呼び出し側の早期 return が漏れてもここで止まる。
func TestInChunks_SkipsExecForAnEmptySlice(t *testing.T) {
	called := false
	if err := inChunks([]int64{}, 1, func([]int64) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("inChunks returned an unexpected error: %v", err)
	}
	if called {
		t.Error("exec was called for an empty slice")
	}
}

// exec がエラーを返したらそこで止めて返すこと。残りを流し続けると、
// 失敗したあとの塊まで書き込まれて中途半端な状態が残る。
func TestInChunks_StopsAtTheFirstError(t *testing.T) {
	size := bulkInsertChunkSize(1)
	rows := make([]int64, size*3)

	wantErr := errors.New("boom")
	calls := 0
	err := inChunks(rows, 1, func([]int64) error {
		calls++
		if calls == 2 {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if calls != 2 {
		t.Errorf("exec calls = %d, want 2 (must stop at the failing chunk)", calls)
	}
}
