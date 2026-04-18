package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomUserRepository interface {
	AddUserToRoom(ctx context.Context, roomID, userID int64) error
	RemoveUserFromRoom(ctx context.Context, roomID, userID int64) error
	GetUserIDsByRoomID(ctx context.Context, roomID int64) ([]int64, error)
	FindDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
}
