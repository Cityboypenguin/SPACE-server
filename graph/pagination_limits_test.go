package graph

import "testing"

// 一度に取得できる件数の上限。
//
// ここが効かなくなっても画面は普通に動くので、壊れても気づけない。行数を決めるのが
// 「データの育ち方」だけになった瞬間に、育った日だけ重くなる。

// TestResolveLimit_IsCapped は limit の頭打ち。
func TestResolveLimit_IsCapped(t *testing.T) {
	huge := int32(1000000)
	if got := resolveLimit(&huge, defaultPageSize, maxPageSize); got != maxPageSize {
		t.Fatalf("limit = %d, want %d", got, maxPageSize)
	}
	if got := resolveLimit(&huge, defaultPageSize, maxMessagePageSize); got != maxMessagePageSize {
		t.Fatalf("チャット履歴 limit = %d, want %d", got, maxMessagePageSize)
	}
}

// TestResolveOffset_IsCapped は offset の頭打ち。
//
// limit を1件に絞っても offset を大きくすれば重くできる
// （MySQL の OFFSET は読み飛ばす行も読んでから捨てる）。しかも返す件数は
// 1件のままなので、クエリの複雑度には現れない。
func TestResolveOffset_IsCapped(t *testing.T) {
	cases := []struct {
		name   string
		offset *int32
		want   int
	}{
		{"未指定", nil, 0},
		{"負数は0へ", ptr32(-5), 0},
		{"範囲内はそのまま", ptr32(100), 100},
		{"上限ちょうど", ptr32(maxOffset), maxOffset},
		{"上限超は頭打ち", ptr32(maxOffset + 1), maxOffset},
		{"桁違いでも頭打ち", ptr32(10000000), maxOffset},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveOffset(c.offset); got != c.want {
				t.Fatalf("offset = %d, want %d", got, c.want)
			}
		})
	}
}

// TestResolvePagination_CapsBothAxes は、一覧の入口が両方の軸を必ず通ること。
func TestResolvePagination_CapsBothAxes(t *testing.T) {
	limit, offset := resolvePagination(ptr32(1000000), ptr32(10000000), defaultPageSize)
	if limit != maxPageSize {
		t.Fatalf("limit = %d, want %d", limit, maxPageSize)
	}
	if offset != maxOffset {
		t.Fatalf("offset = %d, want %d", offset, maxOffset)
	}
}

// TestResolveUnpagedWindow_CapsWhenNoArgumentsAreSent は、あとからページングを
// 足したコレクション（Post.favorites など）が、引数を送られなくても頭打ちに
// なること。
//
// 引数が無いときに全件返していたのが元の姿で、そこが青天井だった。
func TestResolveUnpagedWindow_CapsWhenNoArgumentsAreSent(t *testing.T) {
	q := resolveUnpagedWindow(nil, nil)
	if q.Limit != unpagedCollectionCap {
		t.Fatalf("limit = %d, want %d（引数なしでも頭打ちにすること）", q.Limit, unpagedCollectionCap)
	}
	if q.Offset != 0 {
		t.Fatalf("offset = %d, want 0", q.Offset)
	}

	// 引数を送ってきた場合は、他の一覧と同じ上限に従う。
	q = resolveUnpagedWindow(ptr32(1000000), ptr32(10000000))
	if q.Limit != maxPageSize {
		t.Fatalf("limit = %d, want %d", q.Limit, maxPageSize)
	}
	if q.Offset != maxOffset {
		t.Fatalf("offset = %d, want %d", q.Offset, maxOffset)
	}
}

func ptr32(v int32) *int32 { return &v }
