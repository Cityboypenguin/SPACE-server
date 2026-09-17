package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

func (r *MySQLCourseRoomReadRepository) UpsertLastReadAt(ctx context.Context, roomID, userID, readAt int64) error {
	// GREATEST で既読位置を巻き戻さない（別端末が先に進めていればそちらを残す）。
	now := time.Now().Unix()
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO course_room_reads (room_id, user_id, last_read_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE last_read_at = GREATEST(last_read_at, ?), updated_at = ?
	`, roomID, userID, readAt, now, now, readAt, now)
	return err
}

func (r *MySQLCourseRoomReadRepository) GetLastReadAt(ctx context.Context, roomID, userID int64) (*int64, error) {
	var lastReadAt int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT last_read_at FROM course_room_reads WHERE room_id = ? AND user_id = ?`,
		roomID, userID,
	).Scan(&lastReadAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lastReadAt, nil
}

func (r *MySQLCourseRoomReadRepository) GetLastReadAtByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(roomIDs))
	if len(roomIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(
		`SELECT room_id, last_read_at FROM course_room_reads WHERE user_id = ? AND room_id IN (%s)`,
		placeholders,
	)

	args := make([]interface{}, 0, 1+len(roomIDs))
	args = append(args, userID)
	for _, id := range roomIDs {
		args = append(args, id)
	}

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var roomID, lastReadAt int64
		if err := rows.Scan(&roomID, &lastReadAt); err != nil {
			return nil, err
		}
		result[roomID] = lastReadAt
	}
	return result, rows.Err()
}
