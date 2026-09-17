package room

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type RoomReadStatus struct {
	LastReadAt *int64
	// LastReadMessageID は既読位置そのもの（最後に読んだメッセージID）。
	// まだ一度も読んでいない、または既読位置を時刻でしか持っていない行では nil。
	//
	// UnreadCount と同じ起点なので、クライアントはこれを messages(after:) に渡して
	// 未読ページを取る。時刻（LastReadAt）を起点にすると秒解像度で切ることになり、
	// 「未読件数は1件なのに未読ページは0件」という食い違いが出る。
	LastReadMessageID *int64
	UnreadCount       int
	// PartnerLastReadAt は「相手がどこまで読んだか」。DM でのみ値が入り、
	// それ以外（コミュニティ・授業内チャット）では常に nil。
	//
	// なぜ DM 限定なのか（他人の既読位置を返してよいのは DM だけ、の理由をここに集約する）:
	//   - そもそも「相手」が1人に決まるのは DM だけ。コミュニティで他人の既読を
	//     1つだけ返すと、誰の既読なのかが値から分からない。実際、以前の実装は
	//     メンバーの map を走査して「自分以外の最初の1人」を返しており、Go の map
	//     走査順は不定なので同じ画面を開き直すたびに別人の既読時刻が返りうる状態だった。
	//   - 授業内チャットは匿名なので、誰が読んだかに繋がる情報を出さない
	//     （既読の配信 graph/chat_events.go も授業ルームでは止めている）。
	// コミュニティで「既読したメンバー一覧」を出したくなったら、この1枠ではなく
	// 別のフィールドを足すこと（1枠に押し込むと必ず誰かの既読が消える）。
	PartnerLastReadAt *int64
}

// GetRoomReadStatusUseCase は DM・コミュニティ（room_users に既読位置を持つルーム）の
// 既読状態を返す。授業内チャットは GetCourseRoomReadStatusUseCase。
type GetRoomReadStatusUseCase interface {
	// room には種別まで埋まったルームを渡すこと（PartnerLastReadAt を返してよいかの
	// 判定に Type が要る）。ID だけを渡す形にすると、また人数から DM を推測する
	// 実装に戻ってしまう。
	Execute(ctx context.Context, room *model.Room, userID int64) (*RoomReadStatus, error)
}

type getRoomReadStatusUseCase struct {
	roomUserRepo  repository.RoomUserRepository
	unreadCounter repository.MessageUnreadCounter
}

func NewGetRoomReadStatusUseCase(roomUserRepo repository.RoomUserRepository, unreadCounter repository.MessageUnreadCounter) GetRoomReadStatusUseCase {
	return &getRoomReadStatusUseCase{roomUserRepo: roomUserRepo, unreadCounter: unreadCounter}
}

// Execute は DM・コミュニティの既読位置と未読数を返す。
//
// 未読の起点は repository.UnreadOrigin の規則どおり「既読メッセージID → 既読時刻 →
// 全件」。通常ルームにはフォールバックの時刻が無い（＝まだ一度も読んでいなければ
// 他人のメッセージは全部未読）ので、NewUnreadOrigin には nil を渡す。
func (uc *getRoomReadStatusUseCase) Execute(ctx context.Context, room *model.Room, userID int64) (*RoomReadStatus, error) {
	position, err := uc.roomUserRepo.GetLastRead(ctx, room.ID, userID)
	if err != nil {
		return nil, err
	}

	unreadCount, err := uc.unreadCounter.CountUnreadMessages(ctx, room.ID, userID, repository.NewUnreadOrigin(position, nil))
	if err != nil {
		return nil, err
	}

	var myLastReadAt, myLastReadMessageID *int64
	if position != nil {
		myLastReadAt = position.LastReadAt
		myLastReadMessageID = position.LastReadMessageID
	}

	status := &RoomReadStatus{
		LastReadAt:        myLastReadAt,
		LastReadMessageID: myLastReadMessageID,
		UnreadCount:       unreadCount,
	}

	// DM 以外ではメンバーの既読を引きもしない（使わない値のために毎回1クエリ
	// 増やさない）。PartnerLastReadAt が DM 限定な理由は型のコメント参照。
	if room.Type != model.RoomTypeDM {
		return status, nil
	}

	membersLastReadAt, err := uc.roomUserRepo.GetMembersLastReadAt(ctx, room.ID)
	if err != nil {
		return nil, err
	}
	status.PartnerLastReadAt = partnerLastReadAt(membersLastReadAt, userID)

	return status, nil
}

// partnerLastReadAt は DM のメンバー既読一覧から相手の既読時刻を取り出す。
//
// 「自分以外がちょうど1人」のときだけ値を返し、0人（相手が退会して room_users の行が
// 消えた）や2人以上（データ不整合）では nil を返す。map の走査順は不定なので、
// 複数居るときに先頭を返すと呼ぶたびに別人の既読時刻になる。誰のものか説明できない
// 値を返すくらいなら「相手の既読は分からない」として nil を返すほうが正しい。
func partnerLastReadAt(membersLastReadAt map[int64]*int64, userID int64) *int64 {
	var readAt *int64
	found := 0
	for memberID, at := range membersLastReadAt {
		if memberID == userID {
			continue
		}
		readAt = at
		found++
	}
	if found != 1 {
		return nil
	}
	return readAt
}
