package mysql

import (
	"context"
	"database/sql"
	"strings"
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

func (r *MySQLNotificationRepository) SaveBatch(ctx context.Context, ns []*model.Notification) error {
	if len(ns) == 0 {
		return nil
	}

	now := time.Now()
	placeholders := make([]string, len(ns))
	args := make([]any, 0, len(ns)*8)
	for i, n := range ns {
		n.CreatedAt = now
		placeholders[i] = "(?, ?, ?, ?, ?, ?, ?, ?)"
		args = append(args, n.UserID, n.Type, n.ActorID, n.TargetType, n.TargetID, n.Message, n.IsRead, n.CreatedAt.Unix())
	}

	query := `INSERT INTO notifications (user_id, type, actor_id, target_type, target_id, message, is_read, created_at) VALUES ` +
		strings.Join(placeholders, ", ")

	result, err := r.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}

	firstID, err := result.LastInsertId()
	if err != nil {
		return err
	}

	for i, n := range ns {
		n.ID = firstID + int64(i)
	}

	return nil
}

func (r *MySQLNotificationRepository) ListByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Notification, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ?`, userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, user_id, type, actor_id, target_type, target_id, message, is_read, created_at
		FROM notifications
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		n.CreatedAt = time.Unix(createdAtUnix, 0)
		notifications = append(notifications, &n)
	}
	return notifications, total, rows.Err()
}

func (r *MySQLNotificationRepository) GetByID(ctx context.Context, id int64, userID int64) (*model.Notification, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, user_id, type, actor_id, target_type, target_id, message, is_read, created_at
		FROM notifications
		WHERE id = ? AND user_id = ?
	`, id, userID)

	var n model.Notification
	var createdAtUnix int64
	if err := row.Scan(
		&n.ID, &n.UserID, &n.Type,
		&n.ActorID, &n.TargetType, &n.TargetID,
		&n.Message, &n.IsRead, &createdAtUnix,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	n.CreatedAt = time.Unix(createdAtUnix, 0)
	return &n, nil
}

// notificationGroupKeyExpr は「dm はアクター単位、それ以外は1件ごと」をDB側で表現するための
// グループキー。クライアントに全件を送って集計させるのではなく、ここで集計してから返す。
const notificationGroupKeyExpr = `CASE WHEN type = 'dm' AND actor_id IS NOT NULL THEN CONCAT('dm-', actor_id) ELSE CONCAT('single-', id) END`

func (r *MySQLNotificationRepository) ListGroupedByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.NotificationGroup, int, error) {
	var total int
	countQuery := `
		SELECT COUNT(*) FROM (
			SELECT ` + notificationGroupKeyExpr + ` AS group_key
			FROM notifications
			WHERE user_id = ?
			GROUP BY group_key
		) g
	`
	if err := r.DB.QueryRowContext(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		WITH base AS (
			SELECT id, type, actor_id, target_type, target_id, message, is_read, created_at,
				` + notificationGroupKeyExpr + ` AS group_key
			FROM notifications
			WHERE user_id = ?
		),
		agg AS (
			SELECT group_key,
				COUNT(*) AS cnt,
				SUM(CASE WHEN is_read = 0 THEN 1 ELSE 0 END) AS unread_cnt,
				MAX(created_at) AS latest_created_at
			FROM base
			GROUP BY group_key
		),
		ranked AS (
			SELECT b.*, ROW_NUMBER() OVER (PARTITION BY b.group_key ORDER BY b.created_at DESC, b.id DESC) AS rn
			FROM base b
		)
		SELECT r.type, r.actor_id, r.target_type, r.target_id, r.message, r.id, a.latest_created_at, a.cnt, a.unread_cnt
		FROM ranked r
		JOIN agg a ON r.group_key = a.group_key
		WHERE r.rn = 1
		ORDER BY a.latest_created_at DESC, r.id DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.DB.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var groups []*model.NotificationGroup
	for rows.Next() {
		var g model.NotificationGroup
		var createdAtUnix int64
		if err := rows.Scan(
			&g.Type, &g.ActorID, &g.TargetType, &g.TargetID, &g.Message,
			&g.LatestID, &createdAtUnix, &g.Count, &g.UnreadCount,
		); err != nil {
			return nil, 0, err
		}
		g.CreatedAt = time.Unix(createdAtUnix, 0)
		groups = append(groups, &g)
	}
	return groups, total, rows.Err()
}

func (r *MySQLNotificationRepository) ListByActor(ctx context.Context, userID int64, notifType string, actorID int64, limit, offset int) ([]*model.Notification, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND type = ? AND actor_id = ?`,
		userID, notifType, actorID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, user_id, type, actor_id, target_type, target_id, message, is_read, created_at
		FROM notifications
		WHERE user_id = ? AND type = ? AND actor_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, userID, notifType, actorID, limit, offset)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		n.CreatedAt = time.Unix(createdAtUnix, 0)
		notifications = append(notifications, &n)
	}
	return notifications, total, rows.Err()
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

func (r *MySQLNotificationRepository) MarkAllAsReadByActor(ctx context.Context, userID int64, notifType string, actorID int64) error {
	query := `UPDATE notifications SET is_read = TRUE WHERE user_id = ? AND type = ? AND actor_id = ? AND is_read = FALSE`
	_, err := r.DB.ExecContext(ctx, query, userID, notifType, actorID)
	return err
}

func (r *MySQLNotificationRepository) CountUnread(ctx context.Context, userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = FALSE`
	var count int
	err := r.DB.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

func (r *MySQLNotificationRepository) DeleteReadByUserID(ctx context.Context, userID int64) error {
	query := `DELETE FROM notifications WHERE user_id = ? AND is_read = TRUE`
	_, err := r.DB.ExecContext(ctx, query, userID)
	return err
}

func (r *MySQLNotificationRepository) DeleteReadByActor(ctx context.Context, userID int64, notifType string, actorID int64) error {
	query := `DELETE FROM notifications WHERE user_id = ? AND type = ? AND actor_id = ? AND is_read = TRUE`
	_, err := r.DB.ExecContext(ctx, query, userID, notifType, actorID)
	return err
}

func (r *MySQLNotificationRepository) DeleteByIDs(ctx context.Context, ids []int64, userID int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, userID)
	query := `DELETE FROM notifications WHERE id IN (` + strings.Join(placeholders, ",") + `) AND user_id = ?`
	_, err := r.DB.ExecContext(ctx, query, args...)
	return err
}
