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
		SELECT user_id, bio, image, created_at, updated_at
		FROM profiles
		WHERE user_id = ?
	`
	row := r.DB.QueryRowContext(ctx, query, userID)

	var p model.Profile
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(&p.UserID, &p.Bio, &p.Grade, &p.Image, &createdAtUnix, &updatedAtUnix); err != nil {
	if err := row.Scan(&p.UserID, &p.Bio, &p.Image, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.CreatedAt = time.Unix(createdAtUnix, 0)
	p.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return &p, nil
}

func (r *MySQLProfileRepository) SaveProfile(ctx context.Context, p *model.Profile) error {
	query := `
		INSERT INTO profiles (user_id, bio, image, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		bio = VALUES(bio),
		image = VALUES(image),
		updated_at = VALUES(updated_at)
	`
	_, err := r.DB.ExecContext(ctx, query, p.UserID, p.Bio, p.Grade, p.Image, p.CreatedAt.Unix(), p.UpdatedAt.Unix())
	_, err := r.DB.ExecContext(ctx, query, p.UserID, p.Bio, p.Image, p.CreatedAt, p.UpdatedAt)
	return err
}
