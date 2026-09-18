package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.CourseRoomReadRepository = &MySQLCourseRoomReadRepository{}

type MySQLCourseRoomReadRepository struct {
	DB *sql.DB
}

func NewMySQLCourseRoomReadRepository(db *sql.DB) repository.CourseRoomReadRepository {
	return &MySQLCourseRoomReadRepository{DB: db}
}

// UpsertLastRead は既読位置（メッセージID）と既読時刻をまとめて進める。
// 時刻の列を残すのは GraphQL の roomReadStatus.lastReadAt が表示に使っているため。
// 未読判定に使うのはメッセージIDの方（同じ秒に既読更新と新着が起きたときの
// 取りこぼしを避ける。repository.ReadPosition のコメント参照）。
//
// どちらの列も進める方向にしか動かさない（別端末が先に進めていればそちらを残す）。
func (r *MySQLCourseRoomReadRepository) UpsertLastRead(ctx context.Context, roomID, userID int64, lastReadMessageID *int64, readAt int64) error {
	now := time.Now().Unix()
	query := fmt.Sprintf(`
		INSERT INTO course_room_reads (room_id, user_id, last_read_message_id, last_read_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			last_read_message_id = %s,
			last_read_at = GREATEST(last_read_at, ?),
			updated_at = ?
	`, advanceLastReadMessageIDSQL("last_read_message_id"))
	_, err := r.DB.ExecContext(ctx, query,
		roomID, userID, lastReadMessageID, readAt, now, now,
		lastReadMessageID, readAt, now,
	)
	return err
}

func (r *MySQLCourseRoomReadRepository) GetLastRead(ctx context.Context, roomID, userID int64) (*repository.ReadPosition, error) {
	var messageID, lastReadAt sql.NullInt64
	err := r.DB.QueryRowContext(ctx,
		`SELECT last_read_message_id, last_read_at FROM course_room_reads WHERE room_id = ? AND user_id = ?`,
		roomID, userID,
	).Scan(&messageID, &lastReadAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &repository.ReadPosition{
		LastReadMessageID: nullInt64Ptr(messageID),
		LastReadAt:        nullInt64Ptr(lastReadAt),
	}, nil
}
