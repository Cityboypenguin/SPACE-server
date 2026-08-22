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
// identities, then insert count+1") is not otherwise safe against concurrent first
// posts from two different users in the same room.
func (r *MySQLRoomAnonymousIdentityRepository) GetOrCreate(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	if existing, err := r.find(ctx, roomID, userID); err != nil {
		return nil, err
	} else if existing != nil {
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
	if existing, err := r.find(ctx, roomID, userID); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	var count int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM room_anonymous_identities WHERE room_id = ?`, roomID).Scan(&count); err != nil {
		return nil, err
	}

	label := fmt.Sprintf("匿名%03d", count+1)
	now := time.Now()
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

func (r *MySQLRoomAnonymousIdentityRepository) find(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT id, room_id, user_id, label, created_at FROM room_anonymous_identities WHERE room_id = ? AND user_id = ?`,
		roomID, userID,
	)
	var identity model.RoomAnonymousIdentity
	var createdAt int64
	if err := row.Scan(&identity.ID, &identity.RoomID, &identity.UserID, &identity.Label, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	identity.CreatedAt = time.Unix(createdAt, 0)
	return &identity, nil
}
