package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLSessionRepository struct {
	DB *sql.DB
}

func NewMySQLSessionRepository(db *sql.DB) *MySQLSessionRepository {
	return &MySQLSessionRepository{DB: db}
}

func (r *MySQLSessionRepository) RecordSession(ctx context.Context, userID int64, durationSeconds int, pageViews []repository.PageViewInput) error {
	now := time.Now()
	date := now.Format("2006-01-02")
	nowUnix := now.Unix()

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_session_summaries (user_id, date, session_count, total_duration_seconds, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			session_count = session_count + 1,
			total_duration_seconds = total_duration_seconds + VALUES(total_duration_seconds),
			updated_at = VALUES(updated_at)`,
		userID, date, durationSeconds, nowUnix, nowUnix)
	if err != nil {
		return err
	}

	for _, pv := range pageViews {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO page_view_stats (user_id, date, page_path, view_count, total_duration_seconds, total_max_scroll_depth, created_at, updated_at)
			VALUES (?, ?, ?, 1, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				view_count = view_count + 1,
				total_duration_seconds = total_duration_seconds + VALUES(total_duration_seconds),
				total_max_scroll_depth = total_max_scroll_depth + VALUES(total_max_scroll_depth),
				updated_at = VALUES(updated_at)`,
			userID, date, pv.Path, pv.DurationSeconds, pv.MaxScrollDepth, nowUnix, nowUnix)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
