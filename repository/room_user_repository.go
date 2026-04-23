package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomUserRepository interface {
	AddUserToRoom(ctx context.Context, roomID, userID int64) error
	RemoveUserFromRoom(ctx context.Context, roomID, userID int64) error
	GetUserIDsByRoomID(ctx context.Context, roomID int64) ([]int64, error)
	ListUsersByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64][]*model.User, error)
	ListDMRoomsByUserID(ctx context.Context, userID int64) ([]*model.Room, error)
	FindDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
	FindOrCreateDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
}
