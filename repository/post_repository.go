package repository

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type PostRepository interface {
	CreatePost(ctx context.Context, post *model.Post) (int64, error)
	UpdatePost(ctx context.Context, post *model.Post) error
	DeletePost(ctx context.Context, id int64) (bool, error)
	DeletePostsByUserID(ctx context.Context, userID int64) error
	RecalculateReplyCountsAffectedByUser(ctx context.Context, userID int64) error
	GetPostByID(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByIDs(ctx context.Context, ids []int64) ([]*model.Post, error)
	GetRootPost(ctx context.Context, id int64) (*model.Post, error)
	GetPostByIDIncludeDeleted(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByUserID(ctx context.Context, user_id int64, q PageQuery) ([]*model.Post, int, error)
	GetRepliesByPostIDs(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
	GetRepliesByPostIDsIncludeDeleted(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
	ListTopLevelPosts(ctx context.Context, q PageQuery) ([]*model.Post, int, error)
	GetfollowersTopLevelPostsByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Post, int, error)
	GetFeedPosts(ctx context.Context, viewerID int64, q PageQuery) ([]*model.Post, int, error)
	CountNewFeedPosts(ctx context.Context, viewerID int64, since time.Time) (int, error)
	ListPosts(ctx context.Context, q PageQuery) ([]*model.Post, int, error)
	SearchPosts(ctx context.Context, keyword string) ([]*model.Post, error)
	SearchPostsByHashtag(ctx context.Context, tag string) ([]*model.Post, error)
	CreatePostHashtags(ctx context.Context, postID int64, tags []string) error
	DeletePostHashtagsByPostID(ctx context.Context, postID int64) error
	ListPopularHashtags(ctx context.Context, limit int) ([]*model.HashtagSuggestion, error)
	CountDistinctHashtags(ctx context.Context) (int, error)
	SuggestHashtagsByPrefix(ctx context.Context, prefix string, limit int) ([]*model.HashtagSuggestion, error)
	// CreatePostMentions は投稿に紐づくメンションを一括登録する（重複は無視）。
	CreatePostMentions(ctx context.Context, postID int64, mentions []*model.Mention) error
	DeletePostMentionsByPostID(ctx context.Context, postID int64) error
	// ListMentionsByPostIDs は投稿IDごとのメンション一覧を返す（DataLoader 用）。
	ListMentionsByPostIDs(ctx context.Context, postIDs []int64) (map[int64][]*model.Mention, error)
	GetRepliesByID(ctx context.Context, id int64) ([]*model.Post, error)
	GetFavoritePostsByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Post, int, error)
}
