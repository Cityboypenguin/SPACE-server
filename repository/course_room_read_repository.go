package repository

import "context"

// CourseRoomReadRepository は授業内チャットの既読位置（course_room_reads）を扱う。
//
// 授業内チャットは room_users の membership を使わない（履修していなくても誰でも
// 閲覧できる）ため、既読位置を room_users に持てない（migration 066）。
type CourseRoomReadRepository interface {
	// UpsertLastRead records how far userID has read in a course room.
	// lastReadMessageID はそのルームの最新メッセージID（1件も無ければ nil）、
	// readAt は既読にした時刻。既読位置は戻さない: 別端末が先に進めていれば、
	// そちらを残す。
	UpsertLastRead(ctx context.Context, roomID, userID int64, lastReadMessageID *int64, readAt int64) error
	// GetLastRead returns nil when userID has never read the room.
	GetLastRead(ctx context.Context, roomID, userID int64) (*ReadPosition, error)
}
