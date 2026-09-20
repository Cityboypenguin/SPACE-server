package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type FavoriteRepository interface {
	CreateFavorite(ctx context.Context, favorite *model.Favorite) (int64, error)
	DeleteFavorite(ctx context.Context, id int64) (bool, error)
	DeleteFavoriteByUserIDAndPostID(ctx context.Context, user_id int64, post_id int64) (bool, error)
	GetFavoriteByID(ctx context.Context, id int64) (*model.Favorite, error)
	GetFavoriteByUserIDAndPostID(ctx context.Context, user_id int64, post_id int64) (*model.Favorite, error)
	GetFavoritesByPostID(ctx context.Context, post_id int64) ([]*model.Favorite, error)
	GetFavoritesByUserID(ctx context.Context, user_id int64) ([]*model.Favorite, error)
	// GetFavoritesByPostIDs は投稿ごとのいいねを窓（limit/offset）ぶんだけ引く。
	//
	// 以前は該当する全行を返していた。行数を決めるのがデータの育ち方だけなので、
	// 人気の投稿が出た日に1回のクエリが重くなる（しかも重くなるまで誰も気づけない）。
	// 返信一覧（GetRepliesByPostIDs）と同じく、投稿ごとに窓を切ってから返す。
	GetFavoritesByPostIDs(ctx context.Context, postIDs []int64, q PageQuery) (map[int64][]*model.Favorite, error)

	// CountFavoritesByPostIDs は投稿ごとのいいね件数を1クエリで数える。
	//
	// 表示に要るのは件数と「自分がいいねしたか」だけなのに、以前は
	// GetFavoritesByPostIDs でいいね行を全部取り、その len を件数にしていた。
	// N+1 ではない（バッチ済み）が、人気の投稿ほど運ぶ行が増え、
	// 一覧では「1ページぶんの投稿 × その全いいね」がGraphQLの応答に乗っていた。
	//
	// いいねが0件の投稿は key ごと欠ける（int のゼロ値がそのまま正しい）。
	CountFavoritesByPostIDs(ctx context.Context, postIDs []int64) (map[int64]int, error)

	// ListPostIDsFavoritedBy は postIDs のうち userID がいいねしたものを返す。
	// 件数と同じ理由で、一覧ぶんを1クエリにする。
	// いいねしていない投稿は key ごと欠ける（bool のゼロ値 false が正しい）。
	ListPostIDsFavoritedBy(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error)
}
