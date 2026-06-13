package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MySQLBlockRepository struct {
	DB *sql.DB
}

func NewMySQLBlockRepository(db *sql.DB) *MySQLBlockRepository {
	return &MySQLBlockRepository{DB: db}
}

func (r *MySQLBlockRepository) CreateBlocker(ctx context.Context, block *model.Blocker) (int64, error) {
	now := time.Now()
	block.CreatedAt = now
	query := `
		INSERT INTO blocks (user_id, blocked_user_id, created_at)
		VALUES (?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query,
		block.UserID,
		block.BlockedUserID,
		block.CreatedAt.Unix(),
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	block.ID = id

	return id, nil
}

func (r *MySQLBlockRepository) DeleteBlocker(ctx context.Context, userID int64, blockedID int64) (bool, error) {
	query := "DELETE FROM blocks WHERE user_id = ? AND blocked_user_id = ?"
	result, err := r.DB.ExecContext(ctx, query, userID, blockedID)
	if err != nil {
		return false, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (r *MySQLBlockRepository) ListBlockers(ctx context.Context, userID int64, limit, offset int) ([]*model.Blocker, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blocks WHERE user_id = ?`, userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, user_id, blocked_user_id, created_at FROM blocks WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var blocks []*model.Blocker
	for rows.Next() {
		var block model.Blocker
		var createdAtUnix int64
		if err := rows.Scan(&block.ID, &block.UserID, &block.BlockedUserID, &createdAtUnix); err != nil {
			return nil, 0, err
		}
		block.CreatedAt = time.Unix(createdAtUnix, 0)
		blocks = append(blocks, &block)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return blocks, total, nil
}

func (r *MySQLBlockRepository) GetBlockersByUserID(ctx context.Context, userID int64) ([]*model.Blocker, error) {
	query := "SELECT id, user_id, blocked_user_id, created_at FROM blocks WHERE user_id = ?"
	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []*model.Blocker
	for rows.Next() {
		var block model.Blocker
		var createdAtUnix int64
		if err := rows.Scan(&block.ID, &block.UserID, &block.BlockedUserID, &createdAtUnix); err != nil {
			return nil, err
		}
		block.CreatedAt = time.Unix(createdAtUnix, 0)
		blocks = append(blocks, &block)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return blocks, nil
}

func (r *MySQLBlockRepository) SearchBlockers(ctx context.Context, userID int64, keyword string) ([]*model.Blocker, error) {
	query := `
		SELECT bl.id, bl.user_id, bl.blocked_user_id, bl.created_at
		FROM blocks bl
		JOIN users u ON bl.blocked_user_id = u.id
		WHERE bl.user_id = ? AND (u.name LIKE ? OR u.account_id LIKE ?)
	`

	searchParam := "%" + keyword + "%"
	rows, err := r.DB.QueryContext(ctx, query, userID, searchParam, searchParam)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []*model.Blocker
	for rows.Next() {
		var block model.Blocker
		var createdAtUnix int64
		if err := rows.Scan(
			&block.ID,
			&block.UserID,
			&block.BlockedUserID,
			&createdAtUnix,
		); err != nil {
			return nil, err
		}
		block.CreatedAt = time.Unix(createdAtUnix, 0)
		blocks = append(blocks, &block)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return blocks, nil
}

func (r *MySQLBlockRepository) ExistsBlockRelation(ctx context.Context, userA int64, userB int64) (bool, error) {
	query := `
		SELECT 1 FROM blocks 
		WHERE (user_id = ? AND blocked_user_id = ?) 
		   OR (user_id = ? AND blocked_user_id = ?)
		LIMIT 1
	`
	var dummy int
	err := r.DB.QueryRowContext(ctx, query, userA, userB, userB, userA).Scan(&dummy)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (r *MySQLBlockRepository) GetBlockedAndBlockerIDs(ctx context.Context, userID int64) ([]int64, error) {
	query := `
		SELECT blocked_user_id FROM blocks WHERE user_id = ?
		UNION
		SELECT user_id FROM blocks WHERE blocked_user_id = ?
	`

	rows, err := r.DB.QueryContext(ctx, query, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}
