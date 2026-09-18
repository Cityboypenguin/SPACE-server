package graph

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	postusecase "github.com/Cityboypenguin/SPACE-server/usecase/post"
)

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

// randomCommunities は limit が Int!（必須）なので「未指定なら既定値」が効かず、
// 客が送った値がそのまま ORDER BY RAND() の LIMIT になっていた。他の一覧と同じ
// resolveLimit を通していることを、実行器越しにリポジトリへ届く値で固定する。
type recordingRandomCommunityRepo struct {
	gotLimit int
	repository.CommunityRepository
}

func (r *recordingRandomCommunityRepo) FindRandom(ctx context.Context, userID int64, limit int) ([]*model.Community, error) {
	r.gotLimit = limit
	return nil, nil
}

func TestRandomCommunitiesClampsLimit(t *testing.T) {
	cases := []struct {
		name  string
		limit int32
		want  int
	}{
		{name: "通常値はそのまま", limit: 10, want: 10},
		{name: "過大値は上限で頭打ち", limit: 100000, want: maxPageSize},
		{name: "0 は既定値", limit: 0, want: defaultPageSize},
		{name: "負数は既定値（LIMIT -1 は SQL エラー）", limit: -5, want: defaultPageSize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &recordingRandomCommunityRepo{}
			resolver := &Resolver{
				CommunityUseCases: CommunityUseCases{
					GetRandomCommunitiesUseCase: communityusecase.GetRandomCommunitiesUseCase{CommunityRepo: repo},
				},
			}
			c := newSelectionTestClient(t, resolver, userClaims(1))
			// memberCount / isMember は選ばない。選ぶと集計のユースケース（nil）を呼ぶため。
			var resp map[string]any
			if err := c.Post(`query($l: Int!){ randomCommunities(limit: $l) { ID name } }`, &resp,
				client.Var("l", tc.limit)); err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if repo.gotLimit != tc.want {
				t.Errorf("FindRandom limit = %d, want %d", repo.gotLimit, tc.want)
			}
		})
	}
}

// searchPosts / searchPostsByHashtag は以前は引数そのものが無く、条件に当たった
// 投稿を全件返していた。content LIKE '%...%' は全表走査になるうえ、返す行数も
// 投稿が増えるだけ増えるので、1回の検索が DB とネットワークの両方を占有していた。
//
// 窓を足したので、他の一覧と同じクランプ（resolvePagination）を通ってリポジトリへ
// 届くことを実行器越しに固定する。ここが素通しに戻ると、客が limit: 1000000 を
// 送れば全件走査が復活する。
type recordingPostSearchRepo struct {
	repository.PostRepository

	gotKeyword string
	gotTag     string
	gotQuery   repository.PageQuery
}

func (r *recordingPostSearchRepo) SearchPosts(ctx context.Context, keyword string, q repository.PageQuery) ([]*model.Post, error) {
	r.gotKeyword = keyword
	r.gotQuery = q
	return nil, nil
}

func (r *recordingPostSearchRepo) SearchPostsByHashtag(ctx context.Context, tag string, q repository.PageQuery) ([]*model.Post, error) {
	r.gotTag = tag
	r.gotQuery = q
	return nil, nil
}

func TestSearchPostsClampsTheWindow(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{
			name:      "引数を送らなければ既定値",
			query:     `query{ searchPosts(keyword: "x") { ID } }`,
			wantLimit: defaultPageSize,
		},
		{
			name:       "通常値はそのまま",
			query:      `query{ searchPosts(keyword: "x", limit: 30, offset: 60) { ID } }`,
			wantLimit:  30,
			wantOffset: 60,
		},
		{
			name:      "過大値は上限で頭打ち（全件走査に戻さない）",
			query:     `query{ searchPosts(keyword: "x", limit: 1000000) { ID } }`,
			wantLimit: maxPageSize,
		},
		{
			name:      "0 は既定値",
			query:     `query{ searchPosts(keyword: "x", limit: 0) { ID } }`,
			wantLimit: defaultPageSize,
		},
		{
			name:       "負の offset は 0（LIMIT ? OFFSET -1 は SQL エラー）",
			query:      `query{ searchPosts(keyword: "x", limit: 10, offset: -5) { ID } }`,
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name:      "ハッシュタグ検索も同じ窓を通る",
			query:     `query{ searchPostsByHashtag(tag: "go", limit: 1000000) { ID } }`,
			wantLimit: maxPageSize,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &recordingPostSearchRepo{}
			resolver := &Resolver{
				PostUseCases: PostUseCases{
					SearchPostsUseCase:          postusecase.NewSearchPostsUseCase(repo),
					SearchPostsByHashtagUseCase: postusecase.NewSearchPostsByHashtagUseCase(repo),
				},
			}
			c := newSelectionTestClient(t, resolver, userClaims(1))

			var resp map[string]any
			if err := c.Post(tc.query, &resp); err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if repo.gotQuery.Limit != tc.wantLimit {
				t.Errorf("Limit = %d, want %d", repo.gotQuery.Limit, tc.wantLimit)
			}
			if repo.gotQuery.Offset != tc.wantOffset {
				t.Errorf("Offset = %d, want %d", repo.gotQuery.Offset, tc.wantOffset)
			}
		})
	}
}

// ページングを後付けしたコレクション（返信一覧、Favorite/Block の一覧・検索、
// 管理者の規約一覧）の窓。
//
// スキーマに既定値を入れていないのは、入れると「引数を送っていない既存の
// クライアントで、今まで全件返っていたものが黙って切れる」ため。代わりに
// 引数が無いときはサーバー側の安全弁（unpagedCollectionCap）で頭打ちにする。
// この「既定値ではなく安全弁」という性質をここで固定する。
func TestResolveUnpagedWindow(t *testing.T) {
	cases := []struct {
		name       string
		limit      *int32
		offset     *int32
		wantLimit  int
		wantOffset int
	}{
		{
			name:      "引数なしは安全弁（既定の20件ではない＝今までの見え方を変えない）",
			wantLimit: unpagedCollectionCap,
		},
		{
			name:      "limit を送れば他の一覧と同じ窓になる",
			limit:     int32Ptr(30),
			wantLimit: 30,
		},
		{
			name:      "過大値は maxPageSize で頭打ち（安全弁まで戻さない）",
			limit:     int32Ptr(1000000),
			wantLimit: maxPageSize,
		},
		{
			name:      "0 は未指定と同じ扱い＝安全弁",
			limit:     int32Ptr(0),
			wantLimit: unpagedCollectionCap,
		},
		{
			name:      "負数も安全弁（LIMIT -1 は SQL エラー）",
			limit:     int32Ptr(-5),
			wantLimit: unpagedCollectionCap,
		},
		{
			name:       "offset は素直に通る",
			limit:      int32Ptr(10),
			offset:     int32Ptr(40),
			wantLimit:  10,
			wantOffset: 40,
		},
		{
			name:       "負の offset は 0 に丸める",
			limit:      int32Ptr(10),
			offset:     int32Ptr(-1),
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name:       "limit を送らず offset だけでも窓は安全弁のまま",
			offset:     int32Ptr(5),
			wantLimit:  unpagedCollectionCap,
			wantOffset: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUnpagedWindow(tc.limit, tc.offset)
			if got.Limit != tc.wantLimit {
				t.Errorf("Limit = %d, want %d", got.Limit, tc.wantLimit)
			}
			if got.Offset != tc.wantOffset {
				t.Errorf("Offset = %d, want %d", got.Offset, tc.wantOffset)
			}
			// これらの口はページ型を返さないので total を数える先が無い。
			// WithTotal が立つと、数えない前提のリポジトリで無駄な COUNT が走る。
			if got.WithTotal {
				t.Error("WithTotal = true, want false (ページ型ではないので数える先が無い)")
			}
		})
	}

	// 安全弁は「ページングの上限」より緩いこと。ここが逆転すると、引数を送って
	// いないクライアントが今までより少ない件数しか受け取れなくなる。
	if unpagedCollectionCap <= maxPageSize {
		t.Errorf("unpagedCollectionCap (%d) must be larger than maxPageSize (%d)",
			unpagedCollectionCap, maxPageSize)
	}
}
