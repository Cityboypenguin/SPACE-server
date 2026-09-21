package repository

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// 投稿まわりの口は役割ごとに分けてある。
//
// 分けているのは、利用側（usecase）に「自分が呼ぶもの」だけを受け取らせるため。
// 以前はどのユースケースも PostRepository をまるごと受け取っていたので、
// 投稿を1件引くだけのユースケースが、削除も検索もハッシュタグ登録もできる口を
// 握っていた。何に触りうるかがシグネチャから読めず、偽物を1つ作るのに
// 使いもしないメソッドを揃える必要があり、そして「触れてしまう」ので
// 事故も起こせる。
//
// PostRepository はそれらを束ねたもの。infra 側の実装と DI はこれまでどおり
// 1つの型で足りる（束ねた口を満たしていれば、細い口も自動的に満たす）。

// PostReader は投稿を引く口。
type PostReader interface {
	GetPostByID(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByIDs(ctx context.Context, ids []int64) ([]*model.Post, error)
	GetRootPost(ctx context.Context, id int64) (*model.Post, error)
	GetPostByIDIncludeDeleted(ctx context.Context, id int64) (*model.Post, error)
}

// PostWriter は投稿そのものを作る・変える・消す口。
type PostWriter interface {
	CreatePost(ctx context.Context, post *model.Post) (int64, error)
	UpdatePost(ctx context.Context, post *model.Post) error
	DeletePost(ctx context.Context, id int64) (bool, error)
	DeletePostsByUserID(ctx context.Context, userID int64) error
	RecalculateReplyCountsAffectedByUser(ctx context.Context, userID int64) error
}

// PostLister は一覧・タイムラインを引く口。いずれも窓（PageQuery）を取る。
type PostLister interface {
	GetPostsByUserID(ctx context.Context, user_id int64, q PageQuery) ([]*model.Post, int, error)
	ListTopLevelPosts(ctx context.Context, q PageQuery) ([]*model.Post, int, error)
	GetfollowersTopLevelPostsByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Post, int, error)
	GetFeedPosts(ctx context.Context, viewerID int64, q PageQuery) ([]*model.Post, int, error)
	CountNewFeedPosts(ctx context.Context, viewerID int64, since time.Time) (int, error)
	ListPosts(ctx context.Context, q PageQuery) ([]*model.Post, int, error)
	GetFavoritePostsByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Post, int, error)
}

// PostReplyReader は返信を引く口。
type PostReplyReader interface {
	// 返信一覧は窓（PageQuery）を取る。以前は引数が無く、その投稿の返信を全件
	// 返していた。伸びた投稿ほど重くなるうえ、重くなるまで誰も気づけない。
	// total は返さない（GraphQL 側が [Post!]! を返すので数える先が無い）。
	GetRepliesByID(ctx context.Context, id int64, q PageQuery) ([]*model.Post, error)
	GetRepliesByPostIDs(ctx context.Context, parentIDs []int64, q PageQuery) (map[int64][]*model.Post, error)
	GetRepliesByPostIDsIncludeDeleted(ctx context.Context, parentIDs []int64, q PageQuery) (map[int64][]*model.Post, error)
}

// PostSearcher は投稿の検索。
type PostSearcher interface {
	// 投稿検索は窓（PageQuery）を必ず取る。以前は引数が無く条件に当たった投稿を
	// 全件返していた。content LIKE '%...%' はインデックスが効かず全表走査になるので、
	// 返す行数を絞らないと投稿が増えるだけ1回の検索が重くなる。
	// total は返さない（GraphQL 側が [Post!]! を返すので数える先が無い）ため、
	// PageQuery.WithTotal は見ない。
	SearchPosts(ctx context.Context, keyword string, q PageQuery) ([]*model.Post, error)
	SearchPostsByHashtag(ctx context.Context, tag string, q PageQuery) ([]*model.Post, error)
}

// HashtagRepository は投稿に紐づくハッシュタグ。
type HashtagRepository interface {
	CreatePostHashtags(ctx context.Context, postID int64, tags []string) error
	DeletePostHashtagsByPostID(ctx context.Context, postID int64) error
	ListPopularHashtags(ctx context.Context, limit int) ([]*model.HashtagSuggestion, error)
	CountDistinctHashtags(ctx context.Context) (int, error)
	SuggestHashtagsByPrefix(ctx context.Context, prefix string, limit int) ([]*model.HashtagSuggestion, error)
}

// PostMentionRepository は投稿に紐づくメンション。
type PostMentionRepository interface {
	// CreatePostMentions は投稿に紐づくメンションを一括登録する（重複は無視）。
	CreatePostMentions(ctx context.Context, postID int64, mentions []*model.Mention) error
	DeletePostMentionsByPostID(ctx context.Context, postID int64) error
	// ListMentionsByPostIDs は投稿IDごとのメンション一覧を返す（DataLoader 用）。
	ListMentionsByPostIDs(ctx context.Context, postIDs []int64) (map[int64][]*model.Mention, error)
}

// PostRepository は上記をすべて束ねた口。infra の実装と DI が使う。
//
// usecase がこれを受け取ってよいのは、本当に広く触る場合だけ（投稿の作成・更新は
// 本体・ハッシュタグ・メンションを1つのトランザクションで書くので該当する）。
// 1つ2つしか呼ばないなら、上の細い口のどれかを受け取ること。
type PostRepository interface {
	PostReader
	PostWriter
	PostLister
	PostReplyReader
	PostSearcher
	HashtagRepository
	PostMentionRepository
}
