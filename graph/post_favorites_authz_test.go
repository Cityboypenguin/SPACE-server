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

const favoritingUserID = int64(42)

func postByAuthor(authorID int64) *gqlmodel.Post {
	return &gqlmodel.Post{
		ID:   encodeGraphID("post", 1),
		User: toGraphUser(&model.User{ID: authorID}),
	}
}

// favoritesCtx は FavoriteLoader だけを差したローダーを ctx に積む。
// dataloader.New は全ローダーぶんの口を要求するので、ここでは必要な1本だけを
// 組んだ Loaders を直に渡す。
func favoritesCtx(ctx context.Context) context.Context {
	loader := dataloadgen.NewLoader(
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
	return dataloader.WithLoaders(ctx, &dataloader.Loaders{FavoriteLoader: loader})
}

func TestPostFavorites_RejectsNonAuthor(t *testing.T) {
	r := &postResolver{&Resolver{}}
	ctx := favoritesCtx(auth.WithClaims(context.Background(), userClaims(99)))

	favorites, err := r.Favorites(ctx, postByAuthor(10), nil, nil)
	if err == nil {
		t.Fatalf("他人の投稿のいいね一覧が引けてしまっている（%d 件返った）", len(favorites))
	}
}

func TestPostFavorites_RejectsAnonymous(t *testing.T) {
	r := &postResolver{&Resolver{}}
	ctx := favoritesCtx(context.Background())

	favorites, err := r.Favorites(ctx, postByAuthor(10), nil, nil)
	if err == nil {
		t.Fatalf("未ログインでいいね一覧が引けてしまっている（%d 件返った）", len(favorites))
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
			ctx := favoritesCtx(auth.WithClaims(context.Background(), claims))

			favorites, err := r.Favorites(ctx, postByAuthor(10), nil, nil)
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
