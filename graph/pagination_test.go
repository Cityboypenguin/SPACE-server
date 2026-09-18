package graph

import "testing"

func int32Ptr(v int32) *int32 { return &v }

// ページングの limit/offset は必ず resolvePagination / resolveLimit を通す。
// 生の *int32 を int にして SQL の LIMIT に渡すと、負数（ドライバエラー）も
// 過大値（全件走査）もそのまま DB に届いてしまうため。
func TestResolvePagination(t *testing.T) {
	cases := []struct {
		name       string
		limit      *int32
		offset     *int32
		fallback   int
		wantLimit  int
		wantOffset int
	}{
		{name: "未指定は fallback", fallback: defaultPageSize, wantLimit: defaultPageSize},
		{name: "フィールドごとの fallback を尊重する", fallback: 50, wantLimit: 50},
		{name: "通常値はそのまま", limit: int32Ptr(30), offset: int32Ptr(60), fallback: defaultPageSize, wantLimit: 30, wantOffset: 60},
		{name: "上限で頭打ち", limit: int32Ptr(100000), fallback: defaultPageSize, wantLimit: maxPageSize},
		{name: "ちょうど上限", limit: int32Ptr(maxPageSize), fallback: defaultPageSize, wantLimit: maxPageSize},
		{name: "0 は fallback", limit: int32Ptr(0), fallback: defaultPageSize, wantLimit: defaultPageSize},
		{name: "負の limit は fallback", limit: int32Ptr(-1), fallback: defaultPageSize, wantLimit: defaultPageSize},
		{name: "負の offset は 0", limit: int32Ptr(10), offset: int32Ptr(-5), fallback: defaultPageSize, wantLimit: 10, wantOffset: 0},
		{name: "fallback が上限を超えても頭打ち", limit: nil, fallback: 1000, wantLimit: maxPageSize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLimit, gotOffset := resolvePagination(tc.limit, tc.offset, tc.fallback)
			if gotLimit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", gotLimit, tc.wantLimit)
			}
			if gotOffset != tc.wantOffset {
				t.Errorf("offset = %d, want %d", gotOffset, tc.wantOffset)
			}
		})
	}
}

// チャット履歴だけは上限が maxMessagePageSize（1画面に載る件数が一覧系より多い）。
// クランプの実装自体は resolveLimit に揃っていることを確認する。
func TestResolveLimit_MessagePageHasItsOwnCap(t *testing.T) {
	if got := resolveLimit(int32Ptr(10000), 50, maxMessagePageSize); got != maxMessagePageSize {
		t.Errorf("limit = %d, want %d", got, maxMessagePageSize)
	}
	if got := resolveLimit(nil, 50, maxMessagePageSize); got != 50 {
		t.Errorf("limit = %d, want the messages fallback 50", got)
	}
	if got := resolveLimit(int32Ptr(-3), 50, maxMessagePageSize); got != 50 {
		t.Errorf("limit = %d, want the messages fallback 50 for a negative limit", got)
	}
}
