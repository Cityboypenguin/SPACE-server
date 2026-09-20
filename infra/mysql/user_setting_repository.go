package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.UserSettingRepository = &MySQLUserSettingRepository{}

type MySQLUserSettingRepository struct {
	DB *sql.DB
}

func NewMySQLUserSettingRepository(db *sql.DB) repository.UserSettingRepository {
	return &MySQLUserSettingRepository{DB: db}
}

func (r *MySQLUserSettingRepository) Get(ctx context.Context, userID int64, key string) (string, bool, error) {
	query := "SELECT value FROM user_settings WHERE user_id = ? AND `key` = ? LIMIT 1"
	var value string
	err := r.DB.QueryRowContext(ctx, query, userID, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get user setting: %w", err)
	}
	return value, true, nil
}

func (r *MySQLUserSettingRepository) Set(ctx context.Context, userID int64, key string, value string, updatedAt int64) error {
	query := "INSERT INTO user_settings (user_id, `key`, `value`, updated_at) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value), updated_at = VALUES(updated_at)"
	_, err := r.DB.ExecContext(ctx, query, userID, key, value, updatedAt)
	if err != nil {
		return fmt.Errorf("failed to set user setting: %w", err)
	}
	return nil
}
