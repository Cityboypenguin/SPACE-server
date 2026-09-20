package room

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 授業内チャットの既読位置。授業内チャットは room_users の membership を使わないため、
// 既読位置は course_room_reads（匿名ID room_anonymous_identities とは別表）に
// 持つ。MarkRoomAsReadUseCase / GetRoomReadStatusUseCase
// ではなくこちらを使う。
//
// 置き場が違うだけで、未読の数え方（repository.UnreadOrigin の規則）は通常ルームと
// 共通。違うのはフォールバックだけで、授業ルームは既読位置がまったく無いとき
// 「時間割に登録した時刻」を起点にする。

type MarkCourseRoomAsReadUseCase interface {
	// lastReadMessageID の扱い（検証・nil のときの互換動作）は通常ルームと同じ。
	// 判断は resolveReadMessageID に1箇所だけ置いてある。
	Execute(ctx context.Context, roomID, userID int64, lastReadMessageID *int64) error
}

type markCourseRoomAsReadUseCase struct {
	readRepo      repository.CourseRoomReadRepository
	messageReader repository.MessageReadModel
}

func NewMarkCourseRoomAsReadUseCase(readRepo repository.CourseRoomReadRepository, messageReader repository.MessageReadModel) MarkCourseRoomAsReadUseCase {
	return &markCourseRoomAsReadUseCase{readRepo: readRepo, messageReader: messageReader}
}

func (uc *markCourseRoomAsReadUseCase) Execute(ctx context.Context, roomID, userID int64, lastReadMessageID *int64) error {
	resolved, err := resolveReadMessageID(ctx, uc.messageReader, roomID, lastReadMessageID)
	if err != nil {
		return err
	}
	return uc.readRepo.UpsertLastRead(ctx, roomID, userID, resolved, time.Now().Unix())
}

type GetCourseRoomReadStatusUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error)
}

type getCourseRoomReadStatusUseCase struct {
	readRepo      repository.CourseRoomReadRepository
	unreadCounter repository.MessageUnreadCounter
	courseRepo    repository.CourseRepository
	timetableRepo repository.TimetableRepository
}

func NewGetCourseRoomReadStatusUseCase(
	readRepo repository.CourseRoomReadRepository,
	unreadCounter repository.MessageUnreadCounter,
	courseRepo repository.CourseRepository,
	timetableRepo repository.TimetableRepository,
) GetCourseRoomReadStatusUseCase {
	return &getCourseRoomReadStatusUseCase{
		readRepo:      readRepo,
		unreadCounter: unreadCounter,
		courseRepo:    courseRepo,
		timetableRepo: timetableRepo,
	}
}

// Execute returns the caller's own read position and unread count in a course room.
// PartnerLastReadAt is always nil: course chats are anonymous, so other users' read
// positions are never exposed.
//
// 未読の起点は repository.UnreadOrigin の規則どおり「既読メッセージID → 既読時刻 →
// 時間割に登録した時刻」。授業一覧のバッジ（CountUnreadByCourseRooms）と同じ規則なので、
// 一覧と部屋内で数が食い違わない。まだ一度も開いていない授業で
// 「登録前の過去ログ全部」が未読にならないようにするのが登録時刻起点の狙い。
//
// 時間割に登録していない（読むだけの）ユーザーには起点が無いので、従来どおり
// 他人のメッセージを全件数える。そもそも一覧のバッジにも出てこないため、
// 一覧との食い違いは起きない。
func (uc *getCourseRoomReadStatusUseCase) Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error) {
	position, err := uc.readRepo.GetLastRead(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}

	// フォールバックの時間割登録時刻は、既読位置がまったく無いときにしか使わない。
	// 毎回引くと授業を開くたびに無駄な2クエリ（courses + timetables）が増えるので、
	// 必要になったときだけ引く。
	var fallbackAt *int64
	if position == nil {
		fallbackAt, err = uc.registeredAt(ctx, roomID, userID)
		if err != nil {
			return nil, err
		}
	}

	unreadCount, err := uc.unreadCounter.CountUnreadMessages(ctx, roomID, userID, repository.NewUnreadOrigin(position, fallbackAt))
	if err != nil {
		return nil, err
	}

	var lastReadAt, lastReadMessageID *int64
	if position != nil {
		lastReadAt = position.LastReadAt
		lastReadMessageID = position.LastReadMessageID
	}
	return &RoomReadStatus{
		LastReadAt:        lastReadAt,
		LastReadMessageID: lastReadMessageID,
		UnreadCount:       unreadCount,
	}, nil
}

// registeredAt は roomID の授業を userID が時間割に登録した時刻を返す。
// 未登録・授業が見つからない場合は nil（起点なし＝全件カウント）。
func (uc *getCourseRoomReadStatusUseCase) registeredAt(ctx context.Context, roomID, userID int64) (*int64, error) {
	course, err := uc.courseRepo.GetCourseByRoomID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if course == nil {
		return nil, nil
	}
	return uc.timetableRepo.GetRegisteredAt(ctx, userID, course.ID)
}
