package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.SystemSettingRepository = &MySQLSystemSettingRepository{}

type MySQLSystemSettingRepository struct {
    DB *sql.DB
}

func NewMySQLSystemSettingRepository(db *sql.DB) repository.SystemSettingRepository {
    return &MySQLSystemSettingRepository{DB: db}
}

func (r *MySQLSystemSettingRepository) Get(ctx context.Context, key string) (string, error) {
    query := "SELECT value FROM system_settings WHERE `key` = ? LIMIT 1"
    var value string
    err := r.DB.QueryRowContext(ctx, query, key).Scan(&value)
    if err != nil {
        if err == sql.ErrNoRows {
            return "", nil
        }
        return "", fmt.Errorf("failed to get system setting: %w", err)
    }
    return value, nil
}

func (r *MySQLSystemSettingRepository) GetBool(ctx context.Context, key string) (bool, error) {
    val, err := r.Get(ctx, key)
    if err != nil {
        return false, err
    }
    return val == "true", nil
}

func (r *MySQLSystemSettingRepository) Update(ctx context.Context, key string, value string, updatedAt int64) error {
    query := "UPDATE system_settings SET value = ?, updated_at = ? WHERE `key` = ?"
    _, err := r.DB.ExecContext(ctx, query, value, updatedAt, key)
    if err != nil {
        return fmt.Errorf("failed to update system setting: %w", err)
    }
    return nil
}