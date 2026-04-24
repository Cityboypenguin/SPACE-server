package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MySQLCommunityRepository struct {
	DB *sql.DB
}

func NewMySQLCommunityRepository(db *sql.DB) *MySQLCommunityRepository {
	return &MySQLCommunityRepository{DB: db}
}

func (r *MySQLCommunityRepository) CreateCommunity(ctx context.Context, c *model.Community) (int64, error) {
	query := `
		INSERT INTO communities (name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query, c.Name, c.Description, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *MySQLCommunityRepository) UpdateCommunity(ctx context.Context, c *model.Community) error {
	query := `
		UPDATE communities
		SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.DB.ExecContext(ctx, query, c.Name, c.Description, c.UpdatedAt, c.ID)
	return err
}

func (r *MySQLCommunityRepository) SaveCommunity(ctx context.Context, c *model.Community) error {
	now := time.Now().Unix()
	c.UpdatedAt = now

	if c.ID == 0 {
		c.CreatedAt = now
		id, err := r.CreateCommunity(ctx, c)
		if err != nil {
			return err
		}

		c.ID = id
	} else {
		if err := r.UpdateCommunity(ctx, c); err != nil {
			return err
		}
	}

	return nil
}

func (r *MySQLCommunityRepository) GetCommunityByID(ctx context.Context, id int64) (*model.Community, error) {
	query := `
		SELECT id, name, description, created_at, updated_at
		FROM communities
		WHERE id = ?
	`

	row := r.DB.QueryRowContext(ctx, query, id)

	var c model.Community
	if err := row.Scan(
		&c.ID,
		&c.Name,
		&c.Description,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return &c, nil
}
