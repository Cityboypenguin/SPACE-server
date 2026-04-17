package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLMessageRepository struct {
	DB *sql.DB
}

// UpdateMessage implements [repository.MessageRepository].
func (r *MySQLMessageRepository) UpdateMessage(ctx context.Context, m *model.Message) error {
	panic("unimplemented")
}

func NewMySQLMessageRepository(db *sql.DB) repository.MessageRepository {
	return &MySQLMessageRepository{DB: db}
}

// Implement the methods of the MessageRepository interface here
func (r *MySQLMessageRepository) SaveMessage(ctx context.Context, m *model.Message) error {
	query := "INSERT INTO messages (room_id, user_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?)"
	result, err := r.DB.ExecContext(ctx, query, m.RoomID, m.UserID, m.Content, m.CreatedAt, m.UpdatedAt)
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
	err := row.Scan(&m.ID, &m.RoomID, &m.UserID, &m.Content, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Message not found
		}
		return nil, err
	}
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
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &m.Content, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, &m)
	}
	return messages, nil
}
