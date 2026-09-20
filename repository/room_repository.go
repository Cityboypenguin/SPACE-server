package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomRepository interface {
	SaveRoom(ctx context.Context, r *model.Room) error
	GetRoomByID(ctx context.Context, id int64) (*model.Room, error)
	// GetRoomsByIDs は複数のルームを1クエリでまとめて引く（DataLoader 用）。
	//
	// 返す map には見つかった ID だけを入れる。存在しない ID は key ごと落とす
	// （GetRoomByID が「無ければ nil, nil」を返すのと同じ扱い。呼び出し側は
	// map から引けなかった＝そのルームは無い、と読めばよい）。
	GetRoomsByIDs(ctx context.Context, ids []int64) (map[int64]*model.Room, error)
	DeleteRoom(ctx context.Context, id int64) (bool, error)
}
