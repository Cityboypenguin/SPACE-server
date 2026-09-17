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

type MarkCourseRoomAsReadUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) error
}

type markCourseRoomAsReadUseCase struct {
	readRepo repository.CourseRoomReadRepository
}

func NewMarkCourseRoomAsReadUseCase(readRepo repository.CourseRoomReadRepository) MarkCourseRoomAsReadUseCase {
	return &markCourseRoomAsReadUseCase{readRepo: readRepo}
}

func (uc *markCourseRoomAsReadUseCase) Execute(ctx context.Context, roomID, userID int64) error {
	return uc.readRepo.UpsertLastReadAt(ctx, roomID, userID, time.Now().Unix())
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
// 未読の起点は COALESCE(既読位置, 時間割に登録した時刻)。授業一覧のバッジ
// （CountUnreadByCourseRooms）と同じ起点に揃えてあるので、一覧と部屋内で数が
// 食い違わない。まだ一度も開いていない授業で「登録前の過去ログ全部」が未読に
// ならないようにするのが登録時刻起点の狙い。
//
// 時間割に登録していない（読むだけの）ユーザーには起点が無いので、従来どおり
// 他人のメッセージを全件数える。そもそも一覧のバッジにも出てこないため、
// 一覧との食い違いは起きない。
func (uc *getCourseRoomReadStatusUseCase) Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error) {
	lastReadAt, err := uc.readRepo.GetLastReadAt(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}

	var afterTimestamp int64
	if lastReadAt != nil {
		afterTimestamp = *lastReadAt
	} else {
		registeredAt, err := uc.registeredAt(ctx, roomID, userID)
		if err != nil {
			return nil, err
		}
		if registeredAt != nil {
			afterTimestamp = *registeredAt
		}
	}

	unreadCount, err := uc.unreadCounter.CountUnreadMessages(ctx, roomID, userID, afterTimestamp)
	if err != nil {
		return nil, err
	}

	return &RoomReadStatus{LastReadAt: lastReadAt, UnreadCount: unreadCount}, nil
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
