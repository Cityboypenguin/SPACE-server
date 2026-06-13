package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type PostRepository interface {
	CreatePost(ctx context.Context, post *model.Post) (int64, error)
	UpdatePost(ctx context.Context, post *model.Post) error
	DeletePost(ctx context.Context, id int64) (bool, error)
	DeletePostsByUserID(ctx context.Context, userID int64) error
	GetPostByID(ctx context.Context, id int64) (*model.Post, error)
	GetRootPost(ctx context.Context, id int64) (*model.Post, error)
	GetPostByIDIncludeDeleted(ctx context.Context, id int64) (*model.Post, error)
	GetPostsByUserID(ctx context.Context, user_id int64) ([]*model.Post, error)
	GetRepliesByPostIDs(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
	GetRepliesByPostIDsIncludeDeleted(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error)
	ListTopLevelPosts(ctx context.Context) ([]*model.Post, error)
	ListPosts(ctx context.Context, limit, offset int) ([]*model.Post, int, error)
	SearchPosts(ctx context.Context, keyword string) ([]*model.Post, error)
	GetRepliesByID(ctx context.Context, id int64) ([]*model.Post, error)
}
