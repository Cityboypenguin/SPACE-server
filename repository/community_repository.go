package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type CommunityRepository interface {
	// SaveCommunityWithRoom は Room・RoomUser・Community を単一トランザクションで作成する。
	SaveCommunityWithRoom(ctx context.Context, name, description, avatarKey string, creatorUserID int64) (*model.Community, error)
	GetCommunityByID(ctx context.Context, id int64) (*model.Community, error)
	SearchCommunities(ctx context.Context, name string) ([]*model.Community, error)
	UpdateCommunity(ctx context.Context, c *model.Community) error
	DeleteCommunity(ctx context.Context, id int64) (bool, error)
	ListCommunitiesByUserID(ctx context.Context, userID int64) ([]*model.Community, error)
	ListAllCommunities(ctx context.Context) ([]*model.Community, error)
	// IsSoleOwnerWithOtherMembers は指定ユーザーが他メンバーのいるコミュニティの唯一オーナーかどうかを返す。
	IsSoleOwnerWithOtherMembers(ctx context.Context, userID int64) (bool, error)
	FindRandom(ctx context.Context, userID int64, limit int) ([]*model.Community, error)
}
