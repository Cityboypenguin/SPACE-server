package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetCourseRegistrantIDsUseCase は授業ルームの更新を知らせるべき利用者（履修者）のIDを返す。
//
// GetUserIDsByRoomIDUseCase と分けてあるのは、授業内チャットが room_users を使わない
// 設計だから。あちらは room_users を引くので授業ルームでは宛先が1人も出てこない。
// 母集団が membership ではなく履修登録である、という違いはルーム種別でしか決まらない
// ので、ユースケースを分けて配信側で選ばせる（graph/chat_events.go の
// roomChangedRecipients 参照）。
//
// 返すのはIDの一覧だけで、未読数は含まない。未読数は「必要になった利用者が自分ぶんだけ
// 取得する」もの（myCourseRoomUnreadCounts など）で、送信のたびに全員ぶんを数えて配る
// ものではない。
type GetCourseRegistrantIDsUseCase interface {
	Execute(ctx context.Context, roomID int64) ([]int64, error)
}

var _ GetCourseRegistrantIDsUseCase = &GetCourseRegistrantIDsInteractor{}

type GetCourseRegistrantIDsInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewGetCourseRegistrantIDsUseCase(timetableRepo repository.TimetableRepository) GetCourseRegistrantIDsUseCase {
	return &GetCourseRegistrantIDsInteractor{timetableRepo: timetableRepo}
}

func (uc *GetCourseRegistrantIDsInteractor) Execute(ctx context.Context, roomID int64) ([]int64, error) {
	return uc.timetableRepo.ListRegistrantIDsByCourseRoomID(ctx, roomID)
}
