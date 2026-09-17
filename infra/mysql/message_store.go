package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// repository.MessageReader / repository.MessageWriter の実装。
// メッセージ1件単位の読み書きを担う。

func (r *MySQLMessageRepository) SaveMessage(ctx context.Context, m *model.Message) error {
	db := extractDB(ctx, r.DB)
	content, err := r.encryptContent(m.Content)
	if err != nil {
		return err
	}
	// author_role は投稿時点の立場を焼き付ける（model.NewMessage が既定値を入れる）。
	query := `
		INSERT INTO messages (room_id, user_id, author_role, content, reply_to_message_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := db.ExecContext(ctx, query,
		m.RoomID, m.UserID, m.AuthorRole, content, m.ReplyToID,
		m.CreatedAt.Unix(), m.UpdatedAt.Unix(),
	)
	if err != nil {
		return err
	}
	m.ID, err = result.LastInsertId()
	return err
}

func (r *MySQLMessageRepository) GetMessageByID(ctx context.Context, id int64) (*model.Message, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM messages WHERE id = ? AND deleted_at IS NULL
	`, messageColumns)
	m, err := r.scanMessage(r.DB.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

// GetMessagesByIDs returns the requested messages keyed by ID, skipping any that
// do not exist or have been soft-deleted. 引用返信の返信先をまとめて引くために使う。
func (r *MySQLMessageRepository) GetMessagesByIDs(ctx context.Context, ids []int64) (map[int64]*model.Message, error) {
	result := make(map[int64]*model.Message)
	if len(ids) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	query := fmt.Sprintf(`
		SELECT %s
		FROM messages
		WHERE id IN (%s) AND deleted_at IS NULL
	`, messageColumns, placeholders)

	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		m, err := r.scanMessage(rows)
		if err != nil {
			return nil, err
		}
		result[m.ID] = m
	}
	return result, rows.Err()
}

func (r *MySQLMessageRepository) UpdateMessage(ctx context.Context, m *model.Message) error {
	content, err := r.encryptContent(m.Content)
	if err != nil {
		return err
	}
	query := "UPDATE messages SET content = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL"
	_, err = r.DB.ExecContext(ctx, query, content, m.UpdatedAt.Unix(), m.ID)
	return err
}

// SoftDeleteMessage marks the message as deleted. The WHERE clause requires
// deleted_at IS NULL, so a message that no longer exists or was already
// deleted yields rowsAffected == 0 (returned as false, no error) instead of
// overwriting a prior deletedBy/deletedAt — repeated calls are idempotent.
func (r *MySQLMessageRepository) SoftDeleteMessage(ctx context.Context, id int64, deletedBy int64) (bool, error) {
	query := "UPDATE messages SET deleted_at = ?, deleted_by = ? WHERE id = ? AND deleted_at IS NULL"
	result, err := r.DB.ExecContext(ctx, query, time.Now().Unix(), deletedBy, id)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}
