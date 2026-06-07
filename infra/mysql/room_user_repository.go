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
	now := time.Now().Unix()
	query := "INSERT IGNORE INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)"
	_, err := r.DB.ExecContext(ctx, query, roomID, userID, model.RoomUserRoleMember, now, now)
	return err
}

func (r *MySQLRoomUserRepository) GetRoomUserRole(ctx context.Context, roomID, userID int64) (string, error) {
	var role string
	err := r.DB.QueryRowContext(ctx,
		"SELECT role FROM room_users WHERE room_id = ? AND user_id = ?",
		roomID, userID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return role, nil
}

func (r *MySQLRoomUserRepository) SetRoomUserRole(ctx context.Context, roomID, userID int64, role string) error {
	now := time.Now().Unix()
	_, err := r.DB.ExecContext(ctx,
		"UPDATE room_users SET role = ?, updated_at = ? WHERE room_id = ? AND user_id = ?",
		role, now, roomID, userID,
	)
	return err
}

func (r *MySQLRoomUserRepository) CountRoomUsersByRole(ctx context.Context, roomID int64, role string) (int, error) {
	var count int
	err := r.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM room_users WHERE room_id = ? AND role = ?",
		roomID, role,
	).Scan(&count)
	return count, err
}

func (r *MySQLRoomUserRepository) ListRoomMembersWithRoles(ctx context.Context, roomID int64) ([]*model.RoomMember, error) {
	query := `
		SELECT u.id, u.account_id, u.name, u.email, u.hashed_password, u.role, u.status, u.created_at, u.updated_at, ru.role
		FROM room_users ru
		JOIN users u ON ru.user_id = u.id
		WHERE ru.room_id = ?
		ORDER BY ru.created_at ASC
	`
	rows, err := r.DB.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*model.RoomMember
	for rows.Next() {
		var u model.User
		var roomRole string
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&u.ID, &u.AccountID, &u.Name, &u.Email, &u.HashedPassword,
			&u.Role, &u.Status, &createdAt, &updatedAt, &roomRole,
		); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(createdAt, 0)
		u.UpdatedAt = time.Unix(updatedAt, 0)
		members = append(members, &model.RoomMember{User: &u, Role: roomRole})
	}
	return members, rows.Err()
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
		var createdAt, updatedAt int64
		if err := rows.Scan(&room.ID, &room.Name, &room.Type, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		room.CreatedAt = time.Unix(createdAt, 0)
		room.UpdatedAt = time.Unix(updatedAt, 0)
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
		var createdAt, updatedAt int64
		if err := rows.Scan(
			&roomID,
			&user.ID,
			&user.AccountID,
			&user.Name,
			&user.Email,
			&user.HashedPassword,
			&user.Role,
			&user.Status,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		user.CreatedAt = time.Unix(createdAt, 0)
		user.UpdatedAt = time.Unix(updatedAt, 0)
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
	var createdAt, updatedAt int64
	err := row.Scan(&room.ID, &room.Name, &room.Type, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	room.CreatedAt = time.Unix(createdAt, 0)
	room.UpdatedAt = time.Unix(updatedAt, 0)
	return &room, nil
}

func (r *MySQLRoomUserRepository) UpdateLastReadAt(ctx context.Context, roomID, userID int64, readAt int64) error {
	query := "UPDATE room_users SET last_read_at = ? WHERE room_id = ? AND user_id = ?"
	_, err := r.DB.ExecContext(ctx, query, readAt, roomID, userID)
	return err
}

func (r *MySQLRoomUserRepository) GetLastReadAt(ctx context.Context, roomID, userID int64) (*int64, error) {
	var readAt sql.NullInt64
	err := r.DB.QueryRowContext(ctx,
		"SELECT last_read_at FROM room_users WHERE room_id = ? AND user_id = ?",
		roomID, userID,
	).Scan(&readAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if !readAt.Valid {
		return nil, nil
	}
	v := readAt.Int64
	return &v, nil
}

func (r *MySQLRoomUserRepository) GetMembersLastReadAt(ctx context.Context, roomID int64) (map[int64]*int64, error) {
	query := "SELECT user_id, last_read_at FROM room_users WHERE room_id = ?"
	rows, err := r.DB.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]*int64)
	for rows.Next() {
		var userID int64
		var readAt sql.NullInt64
		if err := rows.Scan(&userID, &readAt); err != nil {
			return nil, err
		}
		if readAt.Valid {
			v := readAt.Int64
			result[userID] = &v
		} else {
			result[userID] = nil
		}
	}
	return result, rows.Err()
}

func (r *MySQLRoomUserRepository) GetLastReadAtByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]*int64, error) {
	if len(roomIDs) == 0 {
		return map[int64]*int64{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(
		"SELECT room_id, last_read_at FROM room_users WHERE user_id = ? AND room_id IN (%s)",
		placeholders,
	)
	args := make([]interface{}, 0, 1+len(roomIDs))
	args = append(args, userID)
	for _, id := range roomIDs {
		args = append(args, id)
	}
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]*int64)
	for rows.Next() {
		var roomID int64
		var readAt sql.NullInt64
		if err := rows.Scan(&roomID, &readAt); err != nil {
			return nil, err
		}
		if readAt.Valid {
			v := readAt.Int64
			result[roomID] = &v
		} else {
			result[roomID] = nil
		}
	}
	return result, rows.Err()
}

func (r *MySQLRoomUserRepository) GetMembersLastReadAtByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]map[int64]*int64, error) {
	if len(roomIDs) == 0 {
		return map[int64]map[int64]*int64{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(
		"SELECT room_id, user_id, last_read_at FROM room_users WHERE room_id IN (%s)",
		placeholders,
	)
	args := make([]interface{}, len(roomIDs))
	for i, id := range roomIDs {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]map[int64]*int64)
	for rows.Next() {
		var roomID, userID int64
		var readAt sql.NullInt64
		if err := rows.Scan(&roomID, &userID, &readAt); err != nil {
			return nil, err
		}
		if result[roomID] == nil {
			result[roomID] = make(map[int64]*int64)
		}
		if readAt.Valid {
			v := readAt.Int64
			result[roomID][userID] = &v
		} else {
			result[roomID][userID] = nil
		}
	}
	return result, rows.Err()
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
	var createdAt, updatedAt int64
	err = row.Scan(&room.ID, &room.Name, &room.Type, &createdAt, &updatedAt)
	if err == nil {
		room.CreatedAt = time.Unix(createdAt, 0)
		room.UpdatedAt = time.Unix(updatedAt, 0)
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return &room, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := time.Now()
	nowUnix := now.Unix()
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
		name, model.RoomTypeDM, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	roomID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	insertMember := "INSERT IGNORE INTO room_users (room_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?)"
	if _, err = tx.ExecContext(ctx, insertMember, roomID, leftUserID, nowUnix, nowUnix); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, insertMember, roomID, rightUserID, nowUnix, nowUnix); err != nil {
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
