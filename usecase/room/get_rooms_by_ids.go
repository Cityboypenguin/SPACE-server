package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetRoomsByIDsUseCase はルームIDの集合からルームをまとめて引く（DataLoader 用）。
//
// 単体の GetRoomUseCase と違い、見つからないルームをエラーにしない。
// 一覧の1件がたまたま消えていても他の行は描けるようにしたいのと、
// DataLoader のバッチ関数は「IDの集合 → map」の形（見つからない ID は key ごと
// 落とす）に揃えているため。
type GetRoomsByIDsUseCase interface {
	Execute(ctx context.Context, ids []int64) (map[int64]*model.Room, error)
}

var _ GetRoomsByIDsUseCase = &GetRoomsByIDsInteractor{}

type GetRoomsByIDsInteractor struct {
	roomRepo repository.RoomRepository
}

func NewGetRoomsByIDsUseCase(roomRepo repository.RoomRepository) GetRoomsByIDsUseCase {
	return &GetRoomsByIDsInteractor{roomRepo: roomRepo}
}

func (uc *GetRoomsByIDsInteractor) Execute(ctx context.Context, ids []int64) (map[int64]*model.Room, error) {
	return uc.roomRepo.GetRoomsByIDs(ctx, ids)
}
