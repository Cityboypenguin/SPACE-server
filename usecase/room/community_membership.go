package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// CountUsersByRoomIDsUseCase はルームの在籍人数をまとめて数える。
// コミュニティ一覧の memberCount 用（1件ずつメンバーIDを取る N+1 の置き換え）。
type CountUsersByRoomIDsUseCase interface {
	Execute(ctx context.Context, roomIDs []int64) (map[int64]int, error)
}

var _ CountUsersByRoomIDsUseCase = &CountUsersByRoomIDsInteractor{}

type CountUsersByRoomIDsInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewCountUsersByRoomIDsUseCase(roomUserRepo repository.RoomUserRepository) CountUsersByRoomIDsUseCase {
	return &CountUsersByRoomIDsInteractor{roomUserRepo: roomUserRepo}
}

func (uc *CountUsersByRoomIDsInteractor) Execute(ctx context.Context, roomIDs []int64) (map[int64]int, error) {
	return uc.roomUserRepo.CountUsersByRoomIDs(ctx, roomIDs)
}

// ListJoinedRoomIDsUseCase は「自分が入っているルーム」を roomIDs の中から返す。
// コミュニティ一覧の isMember 用。
type ListJoinedRoomIDsUseCase interface {
	Execute(ctx context.Context, userID int64, roomIDs []int64) (map[int64]bool, error)
}

var _ ListJoinedRoomIDsUseCase = &ListJoinedRoomIDsInteractor{}

type ListJoinedRoomIDsInteractor struct {
	roomUserRepo repository.RoomUserRepository
}

func NewListJoinedRoomIDsUseCase(roomUserRepo repository.RoomUserRepository) ListJoinedRoomIDsUseCase {
	return &ListJoinedRoomIDsInteractor{roomUserRepo: roomUserRepo}
}

func (uc *ListJoinedRoomIDsInteractor) Execute(ctx context.Context, userID int64, roomIDs []int64) (map[int64]bool, error) {
	return uc.roomUserRepo.ListJoinedRoomIDs(ctx, userID, roomIDs)
}
