package graph

import (
	"context"
	"testing"
	"time"

	"github.com/vikstrous/dataloadgen"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// Post.favorites は「その投稿を誰がいいねしたか」を名前付きで返す唯一の口。
//
// 件数（favoriteCount）と自分の有無（isFavoritedByMe）は誰にでも返すが、名前の
// 一覧は投稿した本人（と管理者）だけに限る。ここが開いていると、投稿の ID さえ
// 分かれば任意の投稿についていいねした人を並べられてしまう。
//
// UI 側で隠すだけでは足りない（GraphQL は誰でも好きなフィールドを選べる）ので、
// 境界はここで固定しておく。

const (
	favoritingUserID = int64(42)
	favoritesPostID  = int64(1)
)

// postRef は GraphQL の親オブジェクトとして渡る投稿。ID しか入れていない。
//
// リゾルバが投稿者を obj.User から読んでいた頃は、ここに User を詰めないと
// nil 参照で落ちた。いまは投稿の行から投稿者を決めるので、ID だけの見本で通る
// ――そして通ることが、graph/presenter.go が作る「ID だけの親投稿」を
// そのまま渡しても壊れない、という保証になっている。
func postRef() *gqlmodel.Post {
	return &gqlmodel.Post{ID: encodeGraphID("post", favoritesPostID)}
}

// favoritesCtx は投稿者が authorID の投稿を1件だけ持つローダーを ctx に積む。
// dataloader.New は全ローダーぶんの口を要求するので、ここでは使う2本だけを
// 組んだ Loaders を直に渡す。
//
// authorID に0を渡すと PostLoader は nil を返す（＝削除済み・ブロック相手で
// 投稿が見えない状態）。
func favoritesCtx(ctx context.Context, authorID int64) context.Context {
	favoriteLoader := dataloadgen.NewLoader(
		func(_ context.Context, keys []dataloader.FavoritePageKey) ([][]*model.Favorite, []error) {
			out := make([][]*model.Favorite, len(keys))
			for i, k := range keys {
				out[i] = []*model.Favorite{{
					ID:        1,
					UserID:    favoritingUserID,
					PostID:    k.PostID,
					CreatedAt: time.Unix(0, 0),
				}}
			}
			return out, make([]error, len(keys))
		},
	)
	postLoader := dataloadgen.NewLoader(
		func(_ context.Context, keys []int64) ([]*model.Post, []error) {
			out := make([]*model.Post, len(keys))
			for i, id := range keys {
				if authorID == 0 {
					continue // 見えない投稿
				}
				out[i] = &model.Post{ID: id, UserID: authorID}
			}
			return out, make([]error, len(keys))
		},
	)
	return dataloader.WithLoaders(ctx, &dataloader.Loaders{
		FavoriteLoader: favoriteLoader,
		PostLoader:     postLoader,
	})
}

func TestPostFavorites_RejectsNonAuthor(t *testing.T) {
	r := &postResolver{&Resolver{}}
	ctx := favoritesCtx(auth.WithClaims(context.Background(), userClaims(99)), 10)

	favorites, err := r.Favorites(ctx, postRef(), nil, nil)
	if err == nil {
		t.Fatalf("他人の投稿のいいね一覧が引けてしまっている（%d 件返った）", len(favorites))
	}
}

func TestPostFavorites_RejectsAnonymous(t *testing.T) {
	r := &postResolver{&Resolver{}}
	ctx := favoritesCtx(context.Background(), 10)

	favorites, err := r.Favorites(ctx, postRef(), nil, nil)
	if err == nil {
		t.Fatalf("未ログインでいいね一覧が引けてしまっている（%d 件返った）", len(favorites))
	}
}

// 削除済み・ブロック相手で投稿そのものが引けないとき、投稿者が分からない以上
// 誰にも返さない。ここが素通りすると、投稿を消した後もいいねした人が残って見える。
func TestPostFavorites_RejectsInvisiblePost(t *testing.T) {
	r := &postResolver{&Resolver{}}
	ctx := favoritesCtx(auth.WithClaims(context.Background(), userClaims(10)), 0)

	favorites, err := r.Favorites(ctx, postRef(), nil, nil)
	if err == nil {
		t.Fatalf("見えない投稿のいいね一覧が引けてしまっている（%d 件返った）", len(favorites))
	}
}

func TestPostFavorites_AllowsAuthorAndAdmin(t *testing.T) {
	cases := map[string]*auth.Claims{
		"投稿した本人": {ID: 10, Role: "user"},
		"管理者":    {ID: 99, Role: "admin"},
	}

	for name, claims := range cases {
		t.Run(name, func(t *testing.T) {
			r := &postResolver{&Resolver{}}
			ctx := favoritesCtx(auth.WithClaims(context.Background(), claims), 10)

			favorites, err := r.Favorites(ctx, postRef(), nil, nil)
			if err != nil {
				t.Fatalf("いいね一覧が引けない: %v", err)
			}
			if len(favorites) != 1 {
				t.Fatalf("favorites = %d 件, want 1", len(favorites))
			}
			if want := encodeGraphID("user", favoritingUserID); favorites[0].User.ID != want {
				t.Fatalf("いいねした人 = %q, want %q", favorites[0].User.ID, want)
			}
		})
	}
}
