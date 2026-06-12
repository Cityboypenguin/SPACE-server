package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MySQLProfileRepository struct {
	DB *sql.DB
}

func NewMySQLProfileRepository(db *sql.DB) *MySQLProfileRepository {
	return &MySQLProfileRepository{DB: db}
}

func (r *MySQLProfileRepository) GetProfileByUserID(ctx context.Context, userID int64) (*model.Profile, error) {
	query := `
		SELECT p.user_id, p.bio,
		       m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at,
		       p.created_at, p.updated_at
		FROM profiles p
		LEFT JOIN media m ON m.id = p.avatar_media_id
		WHERE p.user_id = ?
	`
	row := r.DB.QueryRowContext(ctx, query, userID)

	var p model.Profile
	var createdAtUnix, updatedAtUnix int64
	var mediaID sql.NullInt64
	var mediaUploaderID sql.NullInt64
	var mediaStorageKey sql.NullString
	var mediaContentType sql.NullString
	var mediaCreatedAt sql.NullInt64

	if err := row.Scan(
		&p.UserID, &p.Bio,
		&mediaID, &mediaUploaderID, &mediaStorageKey, &mediaContentType, &mediaCreatedAt,
		&createdAtUnix, &updatedAtUnix,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if mediaID.Valid {
		p.AvatarMedia = &model.Media{
			ID:             mediaID.Int64,
			UploaderUserID: mediaUploaderID.Int64,
			StorageKey:     mediaStorageKey.String,
			ContentType:    mediaContentType.String,
			CreatedAt:      time.Unix(mediaCreatedAt.Int64, 0),
		}
	}

	p.CreatedAt = time.Unix(createdAtUnix, 0)
	p.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &p, nil
}

func (r *MySQLProfileRepository) SaveProfile(ctx context.Context, p *model.Profile) error {
	var avatarMediaID *int64
	if p.AvatarMedia != nil {
		avatarMediaID = &p.AvatarMedia.ID
	}

	query := `
		INSERT INTO profiles (user_id, bio, avatar_media_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		bio = VALUES(bio),
		avatar_media_id = VALUES(avatar_media_id),
		updated_at = VALUES(updated_at)
	`
	_, err := extractDB(ctx, r.DB).ExecContext(ctx, query,
		p.UserID, p.Bio, avatarMediaID,
		p.CreatedAt.Unix(), p.UpdatedAt.Unix(),
	)
	return err
}

func (r *MySQLProfileRepository) SetAvatarMedia(ctx context.Context, userID int64, mediaID int64) error {
	query := `
		INSERT INTO profiles (user_id, bio, avatar_media_id, created_at, updated_at)
		VALUES (?, '', ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		avatar_media_id = VALUES(avatar_media_id),
		updated_at = VALUES(updated_at)
	`
	now := time.Now().Unix()
	_, err := r.DB.ExecContext(ctx, query, userID, mediaID, now, now)
	return err
}

func (r *MySQLProfileRepository) ClearAvatarMedia(ctx context.Context, userID int64) error {
	query := `
		UPDATE profiles SET avatar_media_id = NULL, updated_at = ? WHERE user_id = ?
	`
	_, err := r.DB.ExecContext(ctx, query, time.Now().Unix(), userID)
	return err
}
