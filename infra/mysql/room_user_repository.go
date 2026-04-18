package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLRoomUserRepository struct {
	DB *sql.DB
}

func NewMySQLRoomUserRepository(db *sql.DB) repository.RoomUserRepository {
	return &MySQLRoomUserRepository{DB: db}
}

func (r *MySQLRoomUserRepository) AddUserToRoom(ctx context.Context, roomID, userID int64) error {
	now := time.Now().Unix()
	query := "INSERT IGNORE INTO room_users (room_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?)"
	_, err := r.DB.ExecContext(ctx, query, roomID, userID, now, now)
	return err
}

func (r *MySQLRoomUserRepository) RemoveUserFromRoom(ctx context.Context, roomID, userID int64) error {
	query := "DELETE FROM room_users WHERE room_id = ? AND user_id = ?"
	_, err := r.DB.ExecContext(ctx, query, roomID, userID)
	return err
}

func (r *MySQLRoomUserRepository) GetUserIDsByRoomID(ctx context.Context, roomID int64) ([]int64, error) {
	query := "SELECT user_id FROM room_users WHERE room_id = ?"
	rows, err := r.DB.QueryContext(ctx, query, roomID)
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
	return ids, nil
}

func (r *MySQLRoomUserRepository) FindDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error) {
	query := `
		SELECT r.id, r.name, r.type, r.description, r.created_at, r.updated_at
		FROM rooms r
		JOIN room_users ru1 ON r.id = ru1.room_id AND ru1.user_id = ?
		JOIN room_users ru2 ON r.id = ru2.room_id AND ru2.user_id = ?
		WHERE r.type = 'dm'
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, userID1, userID2)
	var room model.Room
	err := row.Scan(&room.ID, &room.Name, &room.Type, &room.Description, &room.CreatedAt, &room.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &room, nil
}
