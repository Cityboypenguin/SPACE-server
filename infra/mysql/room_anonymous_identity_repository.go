package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

var _ repository.RoomAnonymousIdentityRepository = &MySQLRoomAnonymousIdentityRepository{}

type MySQLRoomAnonymousIdentityRepository struct {
	DB *sql.DB
}

func NewMySQLRoomAnonymousIdentityRepository(db *sql.DB) repository.RoomAnonymousIdentityRepository {
	return &MySQLRoomAnonymousIdentityRepository{DB: db}
}

// GetOrCreate allocates the next sequence number for a room using a MySQL named lock
// (GET_LOCK/RELEASE_LOCK) scoped to the room, since the assignment ("count existing
// labels, then use count+1") is not otherwise safe against concurrent first posts
// from two different users in the same room.
//
// A row may already exist without a label, because reading a course chat records
// last_read_at on the same row (UpsertLastReadAt). Only labeled rows are counted, so
// users who merely read never consume a 匿名NNN number: the numbering keeps following
// the order in which users first post.
func (r *MySQLRoomAnonymousIdentityRepository) GetOrCreate(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	if existing, err := r.find(ctx, roomID, userID); err != nil {
		return nil, err
	} else if existing != nil && existing.Label != "" {
		return existing, nil
	}

	lockName := fmt.Sprintf("room_anon_%d", roomID)
	var acquired int
	if err := r.DB.QueryRowContext(ctx, `SELECT GET_LOCK(?, 5)`, lockName).Scan(&acquired); err != nil {
		return nil, err
	}
	if acquired != 1 {
		return nil, fmt.Errorf("could not acquire anonymous-identity lock for room %d", roomID)
	}
	defer r.DB.ExecContext(context.Background(), `SELECT RELEASE_LOCK(?)`, lockName)

	// Re-check now that we hold the lock: another request may have just created it.
	existing, err := r.find(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Label != "" {
		return existing, nil
	}

	var count int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM room_anonymous_identities WHERE room_id = ? AND label IS NOT NULL`,
		roomID,
	).Scan(&count); err != nil {
		return nil, err
	}

	label := fmt.Sprintf("匿名%03d", count+1)
	now := time.Now()

	// 読むだけで作られた行には、初めて投稿するこのタイミングで番号を割り当てる。
	if existing != nil {
		if _, err := r.DB.ExecContext(ctx,
			`UPDATE room_anonymous_identities SET label = ? WHERE id = ?`,
			label, existing.ID,
		); err != nil {
			return nil, err
		}
		existing.Label = label
		return existing, nil
	}

	result, err := r.DB.ExecContext(ctx,
		`INSERT INTO room_anonymous_identities (room_id, user_id, label, created_at) VALUES (?, ?, ?, ?)`,
		roomID, userID, label, now.Unix(),
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &model.RoomAnonymousIdentity{ID: id, RoomID: roomID, UserID: userID, Label: label, CreatedAt: now}, nil
}

func (r *MySQLRoomAnonymousIdentityRepository) UpsertLastReadAt(ctx context.Context, roomID, userID, readAt int64) error {
	// label は NULL のまま作る（投稿していない利用者に番号を割り当てない）。
	// 既読位置は戻さない: 別端末が先に進めていれば、そちらを残す。
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO room_anonymous_identities (room_id, user_id, label, last_read_at, created_at)
		VALUES (?, ?, NULL, ?, ?)
		ON DUPLICATE KEY UPDATE last_read_at = GREATEST(COALESCE(last_read_at, 0), ?)
	`, roomID, userID, readAt, time.Now().Unix(), readAt)
	return err
}

func (r *MySQLRoomAnonymousIdentityRepository) GetLastReadAt(ctx context.Context, roomID, userID int64) (*int64, error) {
	var lastReadAt sql.NullInt64
	err := r.DB.QueryRowContext(ctx,
		`SELECT last_read_at FROM room_anonymous_identities WHERE room_id = ? AND user_id = ?`,
		roomID, userID,
	).Scan(&lastReadAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !lastReadAt.Valid {
		return nil, nil
	}
	v := lastReadAt.Int64
	return &v, nil
}

// find returns the row for (roomID, userID), with an empty Label when the user has
// read the room but never posted (label is assigned on the first post).
func (r *MySQLRoomAnonymousIdentityRepository) find(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT id, room_id, user_id, label, created_at FROM room_anonymous_identities WHERE room_id = ? AND user_id = ?`,
		roomID, userID,
	)
	var identity model.RoomAnonymousIdentity
	var label sql.NullString
	var createdAt int64
	if err := row.Scan(&identity.ID, &identity.RoomID, &identity.UserID, &label, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	identity.Label = label.String
	identity.CreatedAt = time.Unix(createdAt, 0)
	return &identity, nil
}
