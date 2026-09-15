package room

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// 授業内チャットの既読位置。授業内チャットは room_users の membership を使わないため、
// 既読位置は匿名ID(room_anonymous_identities)の行に持つ。MarkRoomAsReadUseCase /
// GetRoomReadStatusUseCase ではなくこちらを使う。

type MarkCourseRoomAsReadUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) error
}

type markCourseRoomAsReadUseCase struct {
	identityRepo repository.RoomAnonymousIdentityRepository
}

func NewMarkCourseRoomAsReadUseCase(identityRepo repository.RoomAnonymousIdentityRepository) MarkCourseRoomAsReadUseCase {
	return &markCourseRoomAsReadUseCase{identityRepo: identityRepo}
}

func (uc *markCourseRoomAsReadUseCase) Execute(ctx context.Context, roomID, userID int64) error {
	return uc.identityRepo.UpsertLastReadAt(ctx, roomID, userID, time.Now().Unix())
}

type GetCourseRoomReadStatusUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error)
}

type getCourseRoomReadStatusUseCase struct {
	identityRepo repository.RoomAnonymousIdentityRepository
	messageRepo  repository.MessageRepository
}

func NewGetCourseRoomReadStatusUseCase(identityRepo repository.RoomAnonymousIdentityRepository, messageRepo repository.MessageRepository) GetCourseRoomReadStatusUseCase {
	return &getCourseRoomReadStatusUseCase{identityRepo: identityRepo, messageRepo: messageRepo}
}

// Execute returns the caller's own read position and unread count in a course room.
// PartnerLastReadAt is always nil: course chats are anonymous, so other users' read
// positions are never exposed. Unlike the timetable-wide counts
// (CountUnreadByCourseRooms), a room the caller has never opened counts every message
// from others, since this is only used for the caller's own view of one open room.
func (uc *getCourseRoomReadStatusUseCase) Execute(ctx context.Context, roomID, userID int64) (*RoomReadStatus, error) {
	lastReadAt, err := uc.identityRepo.GetLastReadAt(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}

	var afterTimestamp int64
	if lastReadAt != nil {
		afterTimestamp = *lastReadAt
	}
	unreadCount, err := uc.messageRepo.CountUnreadMessages(ctx, roomID, userID, afterTimestamp)
	if err != nil {
		return nil, err
	}

	return &RoomReadStatus{LastReadAt: lastReadAt, UnreadCount: unreadCount}, nil
}
