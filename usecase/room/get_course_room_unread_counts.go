package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetCourseRoomUnreadCountsUseCase は授業ルームの未読数を「その授業を時間割に
// 登録している利用者」ごとに返す（送信者は除く）。未読SSEの宛先を作るための口。
//
// GetMembersUnreadCountsUseCase と分けてあるのは、授業内チャットが room_users を
// 使わない設計だから。あちらは room_users を起点に数えるので、授業ルームでは
// 宛先が1人も出てこない（履修者へ未読のリアルタイム更新が一切届かなくなる）。
// 母集団が membership ではなく履修登録である、という違いはルーム種別でしか
// 決まらないので、ユースケースを分けて呼び出し側（配信側）で選ばせる。
type GetCourseRoomUnreadCountsUseCase interface {
	Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error)
}

type getCourseRoomUnreadCountsUseCase struct {
	unreadCounter repository.MessageUnreadCounter
}

func NewGetCourseRoomUnreadCountsUseCase(unreadCounter repository.MessageUnreadCounter) GetCourseRoomUnreadCountsUseCase {
	return &getCourseRoomUnreadCountsUseCase{unreadCounter: unreadCounter}
}

func (uc *getCourseRoomUnreadCountsUseCase) Execute(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	// 履修者ぶんの宛先をまとめて1クエリで取る（1人ずつ数えると N+1 になる）。
	return uc.unreadCounter.CountUnreadMessagesPerCourseRegistrant(ctx, roomID, excludeUserID)
}
