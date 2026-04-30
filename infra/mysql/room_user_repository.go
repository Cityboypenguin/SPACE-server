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

type MySQLRoomUserRepository struct {
	DB *sql.DB
}

func NewMySQLRoomUserRepository(db *sql.DB) repository.RoomUserRepository {
	return &MySQLRoomUserRepository{DB: db}
}

func (r *MySQLRoomUserRepository) AddUserToRoom(ctx context.Context, roomID, userID int64) error {
	now := time.Now()
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *MySQLRoomUserRepository) ListDMRoomsByUserID(ctx context.Context, userID int64) ([]*model.Room, error) {
	query := `
		SELECT r.id, r.name, r.type, r.created_at, r.updated_at
		FROM rooms r
		JOIN room_users ru ON r.id = ru.room_id
		WHERE ru.user_id = ? AND r.type = 'dm'
		ORDER BY r.updated_at DESC
	`

	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []*model.Room
	for rows.Next() {
		var room model.Room
		if err := rows.Scan(&room.ID, &room.Name, &room.Type, &room.CreatedAt, &room.UpdatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, &room)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rooms, nil
}

func (r *MySQLRoomUserRepository) ListUsersByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64][]*model.User, error) {
	result := make(map[int64][]*model.User)
	if len(roomIDs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(`
		SELECT ru.room_id, u.id, u.account_id, u.name, u.email, u.hashed_password, u.role, u.status, u.created_at, u.updated_at
		FROM room_users ru
		JOIN users u ON ru.user_id = u.id
		WHERE ru.room_id IN (%s)
		ORDER BY ru.room_id ASC, ru.created_at ASC
	`, placeholders)

	args := make([]interface{}, len(roomIDs))
	for i, roomID := range roomIDs {
		args[i] = roomID
	}

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var roomID int64
		var user model.User
		if err := rows.Scan(
			&roomID,
			&user.ID,
			&user.AccountID,
			&user.Name,
			&user.Email,
			&user.HashedPassword,
			&user.Role,
			&user.Status,
			&user.CreatedAt,
			&user.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[roomID] = append(result[roomID], &user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *MySQLRoomUserRepository) FindDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error) {
	query := `
		SELECT r.id, r.name, r.type, r.created_at, r.updated_at
		FROM rooms r
		JOIN room_users ru1 ON r.id = ru1.room_id AND ru1.user_id = ?
		JOIN room_users ru2 ON r.id = ru2.room_id AND ru2.user_id = ?
		WHERE r.type = 'dm'
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, userID1, userID2)
	var room model.Room
	err := row.Scan(&room.ID, &room.Name, &room.Type, &room.CreatedAt, &room.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &room, nil
}

// FindOrCreateDMRoom は2ユーザー間のDMルームをSERIALIZABLEトランザクション内で
// 検索または作成する。並行リクエストによる重複作成を防ぐ。
func (r *MySQLRoomUserRepository) FindOrCreateDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error) {
	leftUserID, rightUserID := userID1, userID2
	if leftUserID > rightUserID {
		leftUserID, rightUserID = rightUserID, leftUserID
	}

	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	findQuery := `
		SELECT r.id, r.name, r.type, r.created_at, r.updated_at
		FROM rooms r
		JOIN room_users ru1 ON r.id = ru1.room_id AND ru1.user_id = ?
		JOIN room_users ru2 ON r.id = ru2.room_id AND ru2.user_id = ?
		WHERE r.type = ?
		LIMIT 1
	`
	row := tx.QueryRowContext(ctx, findQuery, leftUserID, rightUserID, model.RoomTypeDM)
	var room model.Room
	err = row.Scan(&room.ID, &room.Name, &room.Type, &room.CreatedAt, &room.UpdatedAt)
	if err == nil {
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return &room, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := time.Now()
	leftUserName := fmt.Sprintf("user:%d", leftUserID)
	rightUserName := fmt.Sprintf("user:%d", rightUserID)

	userNameQuery := "SELECT name FROM users WHERE id = ? LIMIT 1"
	if err := tx.QueryRowContext(ctx, userNameQuery, leftUserID).Scan(&leftUserName); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, userNameQuery, rightUserID).Scan(&rightUserName); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	name := fmt.Sprintf("DM:%s<->%s", leftUserName, rightUserName)
	result, err := tx.ExecContext(ctx,
		"INSERT INTO rooms (name, type, created_at, updated_at) VALUES (?, ?, ?, ?)",
		name, model.RoomTypeDM, now, now,
	)
	if err != nil {
		return nil, err
	}
	roomID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	insertMember := "INSERT IGNORE INTO room_users (room_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?)"
	if _, err = tx.ExecContext(ctx, insertMember, roomID, leftUserID, now, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, insertMember, roomID, rightUserID, now, now); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return &model.Room{
		ID:        roomID,
		Name:      name,
		Type:      model.RoomTypeDM,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
