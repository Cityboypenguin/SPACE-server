package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// IsRoomMemberUseCase は「この人がこの部屋に入っているか」だけを答える。
//
// GetUserIDsByRoomIDUseCase と分けてあるのは、権限判定が必要としているのが
// 在籍の有無1つだけだから。メンバー一覧で代用すると、判定のたびにルームの
// 人数に比例した行がDBから戻り、メモリに載り、線形探索される。閲覧・購読・
// 編集・削除のすべてが通る経路なので、コミュニティが育つほど効いてくる。
//
// 宛先の一覧そのものが要る経路（送信後の配信・既読通知）は
// GetUserIDsByRoomIDUseCase のままにする。
type IsRoomMemberUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) (bool, error)
}

var _ IsRoomMemberUseCase = &IsRoomMemberInteractor{}

type IsRoomMemberInteractor struct {
	roomUserRepo repository.RoomMembershipReader
}

func NewIsRoomMemberUseCase(roomUserRepo repository.RoomMembershipReader) IsRoomMemberUseCase {
	return &IsRoomMemberInteractor{roomUserRepo: roomUserRepo}
}

func (uc *IsRoomMemberInteractor) Execute(ctx context.Context, roomID, userID int64) (bool, error) {
	return uc.roomUserRepo.IsRoomMember(ctx, roomID, userID)
}
