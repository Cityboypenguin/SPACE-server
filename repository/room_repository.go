package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomRepository interface {
	SaveRoom(ctx context.Context, r *model.Room) error
	GetRoomByID(ctx context.Context, id int64) (*model.Room, error)
	DeleteRoom(ctx context.Context, id int64) (bool, error)
	ListRooms(ctx context.Context) ([]*model.Room, error)
	UpdateRoom(ctx context.Context, r *model.Room) error
}