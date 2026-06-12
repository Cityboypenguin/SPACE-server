package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLCommunityRepository struct {
	DB *sql.DB
}

func NewMySQLCommunityRepository(db *sql.DB) repository.CommunityRepository {
	return &MySQLCommunityRepository{DB: db}
}

// SaveCommunityWithRoom は Room・RoomUser・Community を単一トランザクションで作成する。
// いずれかのステップで失敗した場合はロールバックし、孤立レコードを残さない。
func (r *MySQLCommunityRepository) SaveCommunityWithRoom(ctx context.Context, name, description string, avatarMediaID *int64, creatorUserID int64) (*model.Community, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	nowUnix := now.Unix()

	// 1. rooms に community room を作成
	roomResult, err := tx.ExecContext(ctx,
		`INSERT INTO rooms (name, type, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		name, model.RoomTypeCommunity, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	roomID, err := roomResult.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 2. room_users に作成者をオーナーとして追加
	_, err = tx.ExecContext(ctx,
		`INSERT IGNORE INTO room_users (room_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		roomID, creatorUserID, model.RoomUserRoleOwner, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}

	// 3. communities を作成
	communityResult, err := tx.ExecContext(ctx,
		`INSERT INTO communities (room_id, name, description, avatar_media_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		roomID, name, description, avatarMediaID, nowUnix, nowUnix,
	)
	if err != nil {
		return nil, err
	}
	communityID, err := communityResult.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.Community{
		ID:          communityID,
		RoomID:      roomID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (r *MySQLCommunityRepository) GetCommunityByID(ctx context.Context, id int64) (*model.Community, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT c.id, c.room_id, c.name, c.description,
		       m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at,
		       c.created_at, c.updated_at
		FROM communities c
		LEFT JOIN media m ON m.id = c.avatar_media_id
		WHERE c.id = ?`, id)
	return scanCommunity(row)
}

func (r *MySQLCommunityRepository) SearchCommunities(ctx context.Context, name string) ([]*model.Community, error) {
	query := `
		SELECT c.id, c.room_id, c.name, c.description,
		       m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at,
		       c.created_at, c.updated_at
		FROM communities c
		LEFT JOIN media m ON m.id = c.avatar_media_id
		WHERE c.name LIKE ?
		ORDER BY c.created_at DESC
		LIMIT 50
	`
	rows, err := r.DB.QueryContext(ctx, query, "%"+name+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommunities(rows)
}

func (r *MySQLCommunityRepository) UpdateCommunity(ctx context.Context, c *model.Community) error {
	c.UpdatedAt = time.Now()
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var avatarMediaID *int64
	if c.AvatarMedia != nil {
		avatarMediaID = &c.AvatarMedia.ID
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE communities SET name = ?, description = ?, avatar_media_id = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Description, avatarMediaID, c.UpdatedAt.Unix(), c.ID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE rooms SET name = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.UpdatedAt.Unix(), c.RoomID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *MySQLCommunityRepository) DeleteCommunity(ctx context.Context, id int64) (bool, error) {
	result, err := r.DB.ExecContext(ctx, `DELETE FROM communities WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *MySQLCommunityRepository) ListCommunitiesByUserID(ctx context.Context, userID int64) ([]*model.Community, error) {
	query := `
		SELECT c.id, c.room_id, c.name, c.description,
		       m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at,
		       c.created_at, c.updated_at
		FROM communities c
		LEFT JOIN media m ON m.id = c.avatar_media_id
		JOIN room_users ru ON c.room_id = ru.room_id
		WHERE ru.user_id = ?
		ORDER BY ru.created_at DESC
	`
	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommunities(rows)
}

func (r *MySQLCommunityRepository) ListAllCommunities(ctx context.Context, limit, offset int) ([]*model.Community, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM communities`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT c.id, c.room_id, c.name, c.description,
		       m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at,
		       c.created_at, c.updated_at
		FROM communities c
		LEFT JOIN media m ON m.id = c.avatar_media_id
		ORDER BY c.created_at DESC
		LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	communities, err := scanCommunities(rows)
	if err != nil {
		return nil, 0, err
	}
	return communities, total, nil
}

func (r *MySQLCommunityRepository) IsSoleOwnerWithOtherMembers(ctx context.Context, userID int64) (bool, error) {
	query := `
		SELECT COUNT(*) FROM room_users ru
		JOIN rooms rm ON ru.room_id = rm.id
		WHERE ru.user_id = ? AND ru.role = ? AND rm.type = ?
		AND (SELECT COUNT(*) FROM room_users ru2 WHERE ru2.room_id = ru.room_id AND ru2.role = ?) = 1
		AND (SELECT COUNT(*) FROM room_users ru3 WHERE ru3.room_id = ru.room_id) > 1
	`
	var count int
	err := r.DB.QueryRowContext(ctx, query, userID, model.RoomUserRoleOwner, model.RoomTypeCommunity, model.RoomUserRoleOwner).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *MySQLCommunityRepository) FindRandom(ctx context.Context, userID int64, limit int) ([]*model.Community, error) {
	query := `
		SELECT c.id, c.room_id, c.name, c.description,
		       m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at,
		       c.created_at, c.updated_at
		FROM communities c
		LEFT JOIN media m ON m.id = c.avatar_media_id
		WHERE c.room_id NOT IN (
		    SELECT ru.room_id
		    FROM room_users ru
		    WHERE ru.user_id = ?
		)
		ORDER BY RAND()
		LIMIT ?
	`
	rows, err := r.DB.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCommunities(rows)
}

type communityScanner interface {
	Scan(dest ...any) error
}

func scanCommunity(row communityScanner) (*model.Community, error) {
	var c model.Community
	var createdAt, updatedAt int64
	var mediaID sql.NullInt64
	var mediaUploaderID sql.NullInt64
	var mediaStorageKey sql.NullString
	var mediaContentType sql.NullString
	var mediaCreatedAt sql.NullInt64

	if err := row.Scan(
		&c.ID, &c.RoomID, &c.Name, &c.Description,
		&mediaID, &mediaUploaderID, &mediaStorageKey, &mediaContentType, &mediaCreatedAt,
		&createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if mediaID.Valid {
		c.AvatarMedia = &model.Media{
			ID:             mediaID.Int64,
			UploaderUserID: mediaUploaderID.Int64,
			StorageKey:     mediaStorageKey.String,
			ContentType:    mediaContentType.String,
			CreatedAt:      time.Unix(mediaCreatedAt.Int64, 0),
		}
	}

	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)
	return &c, nil
}

func scanCommunities(rows *sql.Rows) ([]*model.Community, error) {
	var list []*model.Community
	for rows.Next() {
		var c model.Community
		var createdAt, updatedAt int64
		var mediaID sql.NullInt64
		var mediaUploaderID sql.NullInt64
		var mediaStorageKey sql.NullString
		var mediaContentType sql.NullString
		var mediaCreatedAt sql.NullInt64

		if err := rows.Scan(
			&c.ID, &c.RoomID, &c.Name, &c.Description,
			&mediaID, &mediaUploaderID, &mediaStorageKey, &mediaContentType, &mediaCreatedAt,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}

		if mediaID.Valid {
			c.AvatarMedia = &model.Media{
				ID:             mediaID.Int64,
				UploaderUserID: mediaUploaderID.Int64,
				StorageKey:     mediaStorageKey.String,
				ContentType:    mediaContentType.String,
				CreatedAt:      time.Unix(mediaCreatedAt.Int64, 0),
			}
		}

		c.CreatedAt = time.Unix(createdAt, 0)
		c.UpdatedAt = time.Unix(updatedAt, 0)
		list = append(list, &c)
	}
	return list, rows.Err()
}
