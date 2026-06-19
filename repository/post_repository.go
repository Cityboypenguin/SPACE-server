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
	RecalculateReplyCounts(ctx context.Context) error
	RecalculateReplyCountsAffectedByUser(ctx context.Context, userID int64) error
	GetPostByID(ctx context.Context, id int64) (*model.Post, error)
	GetRootPost(ctx context.Context, id int64) (*model.Post, error)
	GetPostByIDIncludeDeleted(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByUserID(ctx context.Context, user_id int64, limit, offset int) ([]*model.Post, int, error)
	GetRepliesByPostIDs(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
	GetRepliesByPostIDsIncludeDeleted(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
	ListTopLevelPosts(ctx context.Context, limit, offset int) ([]*model.Post, int, error)
	GetfollowersTopLevelPostsByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error)
	GetFeedPosts(ctx context.Context, viewerID int64, limit, offset int) ([]*model.Post, int, error)
	CountNewFeedPosts(ctx context.Context, viewerID int64, since time.Time) (int, error)
	ListPosts(ctx context.Context, limit, offset int) ([]*model.Post, int, error)
	SearchPosts(ctx context.Context, keyword string) ([]*model.Post, error)
	GetRepliesByID(ctx context.Context, id int64) ([]*model.Post, error)
	GetFavoritePostsByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error)
}
