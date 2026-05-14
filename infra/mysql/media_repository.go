package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLMediaRepository struct {
	DB *sql.DB
}

func NewMySQLMediaRepository(db *sql.DB) repository.MediaRepository {
	return &MySQLMediaRepository{DB: db}
}

func (r *MySQLMediaRepository) CreateMedia(ctx context.Context, m *model.Media) error {
	db := extractDB(ctx, r.DB)
	query := `
		INSERT INTO media (uploader_user_id, storage_key, content_type, created_at)
		VALUES (?, ?, ?, ?)
	`
	result, err := db.ExecContext(ctx, query,
		m.UploaderUserID, m.StorageKey, m.ContentType, m.CreatedAt.Unix(),
	)
	if err != nil {
		return err
	}
	m.ID, err = result.LastInsertId()
	return err
}

func (r *MySQLMediaRepository) CreatePostMedia(ctx context.Context, postID, mediaID int64, position int) error {
	db := extractDB(ctx, r.DB)
	query := `INSERT INTO post_media (post_id, media_id, position) VALUES (?, ?, ?)`
	_, err := db.ExecContext(ctx, query, postID, mediaID, position)
	return err
}

func (r *MySQLMediaRepository) CreateMessageMedia(ctx context.Context, messageID, mediaID int64, position int) error {
	db := extractDB(ctx, r.DB)
	query := `INSERT INTO message_media (message_id, media_id, position) VALUES (?, ?, ?)`
	_, err := db.ExecContext(ctx, query, messageID, mediaID, position)
	return err
}

func (r *MySQLMediaRepository) ListByPostID(ctx context.Context, postID int64) ([]*model.Media, error) {
	query := `
		SELECT m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at
		FROM media m
		JOIN post_media pm ON pm.media_id = m.id
		WHERE pm.post_id = ?
		ORDER BY pm.position ASC
	`
	rows, err := r.DB.QueryContext(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.Media
	for rows.Next() {
		var m model.Media
		var createdAt int64
		if err := rows.Scan(&m.ID, &m.UploaderUserID, &m.StorageKey, &m.ContentType, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		result = append(result, &m)
	}
	return result, rows.Err()
}

func (r *MySQLMediaRepository) ListByMessageID(ctx context.Context, messageID int64) ([]*model.Media, error) {
	query := `
		SELECT m.id, m.uploader_user_id, m.storage_key, m.content_type, m.created_at
		FROM media m
		JOIN message_media mm ON mm.media_id = m.id
		WHERE mm.message_id = ?
		ORDER BY mm.position ASC
	`
	rows, err := r.DB.QueryContext(ctx, query, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*model.Media
	for rows.Next() {
		var m model.Media
		var createdAt int64
		if err := rows.Scan(&m.ID, &m.UploaderUserID, &m.StorageKey, &m.ContentType, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(createdAt, 0)
		result = append(result, &m)
	}
	return result, rows.Err()
}
