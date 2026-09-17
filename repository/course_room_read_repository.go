package repository

import "context"

// CourseRoomReadRepository は授業内チャットの既読位置（course_room_reads）を扱う。
//
// 授業内チャットは room_users の membership を使わない（誰でも閲覧でき匿名で表示する）
// ため、既読位置を room_users に持てない。匿名ID (room_anonymous_identities) とも
// 別表にしてある: 匿名IDの採番とは寿命も更新頻度も違う関心事で、匿名IDは投稿した人
// だけ・既読位置は読んだ人だけが行を持つため（migration 047 と 066）。
type CourseRoomReadRepository interface {
	// UpsertLastRead records how far userID has read in a course room.
	// lastReadMessageID はそのルームの最新メッセージID（1件も無ければ nil）、
	// readAt は既読にした時刻。既読位置は戻さない: 別端末が先に進めていれば、
	// そちらを残す。
	UpsertLastRead(ctx context.Context, roomID, userID int64, lastReadMessageID *int64, readAt int64) error
	// GetLastRead returns nil when userID has never read the room.
	GetLastRead(ctx context.Context, roomID, userID int64) (*ReadPosition, error)
	// GetLastReadAtByRoomIDs は複数ルームの既読位置をまとめて返す（未読の部屋は
	// キーごと含まれない）。授業一覧のように部屋数ぶん引くと N+1 になる用途向け。
	GetLastReadAtByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int64, error)
}
