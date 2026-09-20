package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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
	query := "INSERT INTO rooms (name, type, created_at, updated_at) VALUES (?, ?, ?, ?)"
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, query, room.Name, room.Type, room.CreatedAt.Unix(), room.UpdatedAt.Unix())
	if err != nil {
		return err
	}
	room.ID, err = result.LastInsertId()
	return err
}

func (r *MySQLRoomRepository) GetRoomByID(ctx context.Context, id int64) (*model.Room, error) {
	query := "SELECT id, name, type, created_at, updated_at FROM rooms WHERE id = ?"
	row := extractDB(ctx, r.DB).QueryRowContext(ctx, query, id)

	var room model.Room
	var createdAt, updatedAt int64
	err := row.Scan(&room.ID, &room.Name, &room.Type, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // Room not found
		}
		return nil, err
	}
	room.CreatedAt = time.Unix(createdAt, 0)
	room.UpdatedAt = time.Unix(updatedAt, 0)
	return &room, nil
}

func (r *MySQLRoomRepository) DeleteRoom(ctx context.Context, id int64) (bool, error) {
	query := "DELETE FROM rooms WHERE id = ?"
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

// GetRoomsByIDs は GetRoomByID の一括版。DataLoader から1クエリでまとめて呼ばれる。
//
// 見つからなかった ID は map に入れない（単体版が nil, nil を返すのと同じ扱い）。
// 「見つからない」をエラーにしないのは、ルームが消えていても一覧の他の行は
// 描けるようにするため。
func (r *MySQLRoomRepository) GetRoomsByIDs(ctx context.Context, ids []int64) (map[int64]*model.Room, error) {
	result := make(map[int64]*model.Room, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	query := fmt.Sprintf(
		"SELECT id, name, type, created_at, updated_at FROM rooms WHERE id IN (%s)",
		placeholders,
	)

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var room model.Room
		var createdAt, updatedAt int64
		if err := rows.Scan(&room.ID, &room.Name, &room.Type, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		room.CreatedAt = time.Unix(createdAt, 0)
		room.UpdatedAt = time.Unix(updatedAt, 0)
		result[room.ID] = &room
	}
	return result, rows.Err()
}
