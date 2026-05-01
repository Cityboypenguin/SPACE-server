package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLMessageRepository struct {
	DB *sql.DB
}

// UpdateMessage implements [repository.MessageRepository].
func (r *MySQLMessageRepository) UpdateMessage(ctx context.Context, m *model.Message) error {
	query := "UPDATE messages SET content = ?, updated_at = ? WHERE id = ?"
	_, err := r.DB.ExecContext(ctx, query, m.Content, m.UpdatedAt.Unix(), m.ID)
	return err
}

func NewMySQLMessageRepository(db *sql.DB) repository.MessageRepository {
	return &MySQLMessageRepository{DB: db}
}

// Implement the methods of the MessageRepository interface here
func (r *MySQLMessageRepository) SaveMessage(ctx context.Context, m *model.Message) error {
	query := "INSERT INTO messages (room_id, user_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?)"
	result, err := r.DB.ExecContext(ctx, query, m.RoomID, m.UserID, m.Content, m.CreatedAt.Unix(), m.UpdatedAt.Unix())
	if err != nil {
		return err
	}
	m.ID, err = result.LastInsertId()
	return err
}

func (r *MySQLMessageRepository) GetMessageByID(ctx context.Context, id int64) (*model.Message, error) {
	query := "SELECT id, room_id, user_id, content, created_at, updated_at FROM messages WHERE id = ?"
	row := r.DB.QueryRowContext(ctx, query, id)

	var m model.Message
	var createdAt, updatedAt int64
	err := row.Scan(&m.ID, &m.RoomID, &m.UserID, &m.Content, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	m.CreatedAt = time.Unix(createdAt, 0)
	m.UpdatedAt = time.Unix(updatedAt, 0)
	return &m, nil
}

func (r *MySQLMessageRepository) DeleteMessage(ctx context.Context, id int64) (bool, error) {
	query := "DELETE FROM messages WHERE id = ?"
	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (r *MySQLMessageRepository) ListMessagesByRoomID(ctx context.Context, roomID int64) ([]*model.Message, error) {
	query := "SELECT id, room_id, user_id, content, created_at, updated_at FROM messages WHERE room_id = ?"
	rows, err := r.DB.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*model.Message
	for rows.Next() {
		var m model.Message
		var createdAt, updatedAt int64
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &m.Content, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		m.UpdatedAt = time.Unix(updatedAt, 0)
		messages = append(messages, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}
