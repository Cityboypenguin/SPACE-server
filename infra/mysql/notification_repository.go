package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLNotificationRepository struct {
	DB *sql.DB
}

func NewMySQLNotificationRepository(db *sql.DB) repository.NotificationRepository {
	return &MySQLNotificationRepository{DB: db}
}

func (r *MySQLNotificationRepository) Save(ctx context.Context, n *model.Notification) error {
	n.CreatedAt = time.Now()
	query := `
		INSERT INTO notifications (user_id, type, actor_id, target_type, target_id, message, is_read, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query,
		n.UserID, n.Type, n.ActorID, n.TargetType, n.TargetID,
		n.Message, n.IsRead, n.CreatedAt.Unix(),
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	n.ID = id
	return nil
}

func (r *MySQLNotificationRepository) ListByUserID(ctx context.Context, userID int64, limit int) ([]*model.Notification, error) {
	query := `
		SELECT id, user_id, type, actor_id, target_type, target_id, message, is_read, created_at
		FROM notifications
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := r.DB.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []*model.Notification
	for rows.Next() {
		var n model.Notification
		var createdAtUnix int64
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Type,
			&n.ActorID, &n.TargetType, &n.TargetID,
			&n.Message, &n.IsRead, &createdAtUnix,
		); err != nil {
			return nil, err
		}
		n.CreatedAt = time.Unix(createdAtUnix, 0)
		notifications = append(notifications, &n)
	}
	return notifications, nil
}

func (r *MySQLNotificationRepository) MarkAsRead(ctx context.Context, id int64, userID int64) error {
	query := `UPDATE notifications SET is_read = TRUE WHERE id = ? AND user_id = ?`
	_, err := r.DB.ExecContext(ctx, query, id, userID)
	return err
}

func (r *MySQLNotificationRepository) MarkAllAsRead(ctx context.Context, userID int64) error {
	query := `UPDATE notifications SET is_read = TRUE WHERE user_id = ? AND is_read = FALSE`
	_, err := r.DB.ExecContext(ctx, query, userID)
	return err
}

func (r *MySQLNotificationRepository) CountUnread(ctx context.Context, userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = FALSE`
	var count int
	err := r.DB.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}
