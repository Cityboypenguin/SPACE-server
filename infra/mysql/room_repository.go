package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLRoomRepository struct {
	DB *sql.DB
}

func NewMySQLRoomRepository(db *sql.DB) repository.RoomRepository {
	return &MySQLRoomRepository{DB: db}
}

func (r *MySQLRoomRepository) SaveRoom(ctx context.Context, room *model.Room) error {
	query := "INSERT INTO rooms (name, type, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)"
	result, err := r.DB.ExecContext(ctx, query, room.Name, room.Type, room.Description, room.CreatedAt, room.UpdatedAt)
	if err != nil {
		return err
	}
	room.ID, err = result.LastInsertId()
	return err
}

func (r *MySQLRoomRepository) GetRoomByID(ctx context.Context, id int64) (*model.Room, error) {
	query := "SELECT id, name, type, description, created_at, updated_at FROM rooms WHERE id = ?"
	row := r.DB.QueryRowContext(ctx, query, id)

	var room model.Room
	err := row.Scan(&room.ID, &room.Name, &room.Type, &room.Description, &room.CreatedAt, &room.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Room not found
		}
		return nil, err
	}
	return &room, nil
}

func (r *MySQLRoomRepository) DeleteRoom(ctx context.Context, id int64) (bool, error) {
	query := "DELETE FROM rooms WHERE id = ?"
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

func (r *MySQLRoomRepository) ListRooms(ctx context.Context) ([]*model.Room, error) {
	query := "SELECT id, name, type, description, created_at, updated_at FROM rooms"
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []*model.Room
	for rows.Next() {
		var room model.Room
		if err := rows.Scan(&room.ID, &room.Name, &room.Type, &room.Description, &room.CreatedAt, &room.UpdatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, &room)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rooms, nil
}

func (r *MySQLRoomRepository) UpdateRoom(ctx context.Context, room *model.Room) error {
	query := "UPDATE rooms SET name = ?, description = ?, updated_at = ? WHERE id = ?"
	_, err := r.DB.ExecContext(ctx, query, room.Name, room.Description, room.UpdatedAt, room.ID)
	return err
}