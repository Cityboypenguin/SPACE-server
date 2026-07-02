package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/messagecrypto"
	"github.com/Cityboypenguin/SPACE-server/model"
)

type MySQLMessageRepository struct {
	DB     *sql.DB
	cipher *messagecrypto.Cipher
}

func NewMySQLMessageRepository(db *sql.DB) (*MySQLMessageRepository, error) {
	cipher, err := messagecrypto.New(os.Getenv("MESSAGE_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	return &MySQLMessageRepository{DB: db, cipher: cipher}, nil
}

func (r *MySQLMessageRepository) SaveMessage(ctx context.Context, m *model.Message) error {
	db := extractDB(ctx, r.DB)
	content, err := r.encryptContent(m.Content)
	if err != nil {
		return err
	}
	query := `
		INSERT INTO messages (room_id, user_id, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := db.ExecContext(ctx, query,
		m.RoomID, m.UserID, content,
		m.CreatedAt.Unix(), m.UpdatedAt.Unix(),
	)
	if err != nil {
		return err
	}
	m.ID, err = result.LastInsertId()
	return err
}

func (r *MySQLMessageRepository) GetMessageByID(ctx context.Context, id int64) (*model.Message, error) {
	query := `
		SELECT id, room_id, user_id, content, created_at, updated_at
		FROM messages WHERE id = ?
	`
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
	if err := r.decryptMessage(&m); err != nil {
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

func (r *MySQLMessageRepository) EncryptPlaintextMessages(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 500
	}

	total := 0
	for {
		rows, err := r.DB.QueryContext(ctx, `
			SELECT id, content
			FROM messages
			WHERE content NOT LIKE ?
			ORDER BY id ASC
			LIMIT ?
		`, messagecrypto.Prefix+"%", batchSize)
		if err != nil {
			return total, err
		}

		type messageContent struct {
			id      int64
			content string
		}
		var batch []messageContent
		for rows.Next() {
			var item messageContent
			if err := rows.Scan(&item.id, &item.content); err != nil {
				rows.Close()
				return total, err
			}
			batch = append(batch, item)
		}
		if err := rows.Close(); err != nil {
			return total, err
		}
		if err := rows.Err(); err != nil {
			return total, err
		}
		if len(batch) == 0 {
			return total, nil
		}

		for _, item := range batch {
			encrypted, err := r.encryptContent(item.content)
			if err != nil {
				return total, err
			}
			result, err := r.DB.ExecContext(ctx, `
				UPDATE messages
				SET content = ?
				WHERE id = ? AND content = ?
			`, encrypted, item.id, item.content)
			if err != nil {
				return total, err
			}
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return total, err
			}
			total += int(rowsAffected)
		}
	}
}

func (r *MySQLMessageRepository) ListMessagesByRoomID(ctx context.Context, roomID int64, limit int, beforeID *int64, afterID *int64, afterTime *time.Time) ([]*model.Message, bool, bool, error) {
	var query string
	var args []interface{}
	var ascOrder bool

	switch {
	case afterTime != nil:
		// 未読起点: afterTime より新しいメッセージを昇順で取得
		query = `SELECT id, room_id, user_id, content, created_at, updated_at
			FROM messages WHERE room_id = ? AND created_at > ? ORDER BY id ASC LIMIT ?`
		args = []interface{}{roomID, afterTime.Unix(), limit + 1}
		ascOrder = true
	case afterID != nil:
		// 新着ページング: afterID より新しいメッセージを昇順で取得
		query = `SELECT id, room_id, user_id, content, created_at, updated_at
			FROM messages WHERE room_id = ? AND id > ? ORDER BY id ASC LIMIT ?`
		args = []interface{}{roomID, *afterID, limit + 1}
		ascOrder = true
	case beforeID != nil:
		// 過去ページング: beforeID より古いメッセージを降順で取得して反転
		query = `SELECT id, room_id, user_id, content, created_at, updated_at
			FROM messages WHERE room_id = ? AND id < ? ORDER BY id DESC LIMIT ?`
		args = []interface{}{roomID, *beforeID, limit + 1}
		ascOrder = false
	default:
		// 初回ロード（未読なし）: 最新メッセージを降順で取得して反転
		query = `SELECT id, room_id, user_id, content, created_at, updated_at
			FROM messages WHERE room_id = ? ORDER BY id DESC LIMIT ?`
		args = []interface{}{roomID, limit + 1}
		ascOrder = false
	}

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, false, err
	}
	defer rows.Close()

	var messages []*model.Message
	for rows.Next() {
		var m model.Message
		var createdAt, updatedAt int64
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &m.Content, &createdAt, &updatedAt); err != nil {
			return nil, false, false, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		m.UpdatedAt = time.Unix(updatedAt, 0)
		if err := r.decryptMessage(&m); err != nil {
			return nil, false, false, err
		}
		messages = append(messages, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, false, err
	}

	hasExtraRow := len(messages) > limit
	if hasExtraRow {
		messages = messages[:limit]
	}

	if !ascOrder {
		// DESC で取得したので昇順（古い順）に戻す
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}

	var hasMoreBefore, hasMoreAfter bool
	switch {
	case afterTime != nil || afterID != nil:
		hasMoreBefore = true // after 系は常に古いメッセージが存在する
		hasMoreAfter = hasExtraRow
	case beforeID != nil:
		hasMoreBefore = hasExtraRow
		hasMoreAfter = true // before 系は常に新しいメッセージが存在する
	default:
		hasMoreBefore = hasExtraRow
		hasMoreAfter = false // 最新端なので after はなし（WebSocket が担う）
	}

	return messages, hasMoreBefore, hasMoreAfter, nil
}

func (r *MySQLMessageRepository) UpdateMessage(ctx context.Context, m *model.Message) error {
	content, err := r.encryptContent(m.Content)
	if err != nil {
		return err
	}
	query := "UPDATE messages SET content = ?, updated_at = ? WHERE id = ?"
	_, err = r.DB.ExecContext(ctx, query, content, m.UpdatedAt.Unix(), m.ID)
	return err
}

func (r *MySQLMessageRepository) CountUnreadMessages(ctx context.Context, roomID, userID int64, afterTimestamp int64) (int, error) {
	query := `
		SELECT COUNT(*) FROM messages
		WHERE room_id = ? AND user_id != ? AND created_at > ?
	`
	var count int
	err := r.DB.QueryRowContext(ctx, query, roomID, userID, afterTimestamp).Scan(&count)
	return count, err
}

func (r *MySQLMessageRepository) CountUnreadMessagesByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int)
	if len(roomIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(`
		SELECT m.room_id, COUNT(*) as unread_count
		FROM messages m
		JOIN room_users ru ON ru.room_id = m.room_id AND ru.user_id = ?
		WHERE m.room_id IN (%s)
		  AND m.user_id != ?
		  AND (ru.last_read_at IS NULL OR m.created_at > ru.last_read_at)
		GROUP BY m.room_id
	`, placeholders)

	args := make([]interface{}, 0, 2+len(roomIDs))
	args = append(args, userID)
	for _, id := range roomIDs {
		args = append(args, id)
	}
	args = append(args, userID)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var roomID int64
		var count int
		if err := rows.Scan(&roomID, &count); err != nil {
			return nil, err
		}
		result[roomID] = count
	}
	return result, rows.Err()
}

func (r *MySQLMessageRepository) CountUnreadMessagesByRoomType(ctx context.Context, userID int64, roomType string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM messages m
		JOIN room_users ru ON ru.room_id = m.room_id AND ru.user_id = ?
		JOIN rooms r ON r.id = m.room_id AND r.type = ?
		WHERE m.user_id != ?
		  AND (ru.last_read_at IS NULL OR m.created_at > ru.last_read_at)
	`
	var count int
	err := r.DB.QueryRowContext(ctx, query, userID, roomType, userID).Scan(&count)
	return count, err
}

func (r *MySQLMessageRepository) GetLastMessagesByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error) {
	result := make(map[int64]*model.Message)
	if len(roomIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(roomIDs)), ",")
	query := fmt.Sprintf(`
		SELECT m.id, m.room_id, m.user_id, m.content, m.created_at, m.updated_at
		FROM messages m
		INNER JOIN (
			SELECT room_id, MAX(id) AS max_id
			FROM messages
			WHERE room_id IN (%s)
			GROUP BY room_id
		) latest ON m.room_id = latest.room_id AND m.id = latest.max_id
	`, placeholders)

	args := make([]interface{}, len(roomIDs))
	for i, id := range roomIDs {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var m model.Message
		var createdAt, updatedAt int64
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &m.Content, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		m.UpdatedAt = time.Unix(updatedAt, 0)
		if err := r.decryptMessage(&m); err != nil {
			return nil, err
		}
		result[m.RoomID] = &m
	}
	return result, rows.Err()
}

func (r *MySQLMessageRepository) encryptContent(content string) (string, error) {
	encrypted, err := r.cipher.Encrypt(content)
	if err != nil {
		return "", fmt.Errorf("encrypt message content: %w", err)
	}
	return encrypted, nil
}

func (r *MySQLMessageRepository) decryptMessage(m *model.Message) error {
	content, err := r.cipher.Decrypt(m.Content)
	if err != nil {
		return fmt.Errorf("decrypt message content: %w", err)
	}
	m.Content = content
	return nil
}

func (r *MySQLMessageRepository) CountUnreadMessagesPerMember(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error) {
	query := `
		SELECT ru.user_id, COUNT(m.id) as unread_count
		FROM room_users ru
		LEFT JOIN messages m ON m.room_id = ru.room_id
		  AND m.user_id != ru.user_id
		  AND m.created_at > COALESCE(ru.last_read_at, 0)
		WHERE ru.room_id = ?
		  AND ru.user_id != ?
		GROUP BY ru.user_id
	`
	rows, err := r.DB.QueryContext(ctx, query, roomID, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var userID int64
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}
		result[userID] = count
	}
	return result, rows.Err()
}
