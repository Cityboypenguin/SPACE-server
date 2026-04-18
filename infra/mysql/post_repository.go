package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLPostRepository struct {
	DB *sql.DB
}

func NewMySQLPostRepository(db *sql.DB) repository.PostRepository {
	return &MySQLPostRepository{DB: db}
}

func (r *MySQLPostRepository) GetPostByID(ctx context.Context, id int64) (*model.Post, error) {
	query := `
		SELECT id, user_id, title, content, created_at, updated_at
		FROM posts
		WHERE id = ?
	`
	row := r.DB.QueryRowContext(ctx, query, id)

	var p model.Post
	if err := row.Scan(&p.ID, &p.UserID, &p.Content, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}

	return &p, nil
}

func (r *MySQLPostRepository) SavePost(ctx context.Context, p *model.Post) error {
	now := time.Now()
	p.UpdatedAt = now

	if p.ID == 0 {
		p.CreatedAt = now
		id, err := r.CreatePost(ctx, p)
		if err != nil {
			return err
		}

		p.ID = id
	} else {
		if err := r.UpdatePost(ctx, p); err != nil {
			return err
		}
	}

	return nil
}

func (r *MySQLPostRepository) CreatePost(ctx context.Context, p *model.Post) (int64, error) {
	query := `
		INSERT INTO posts (user_id, title, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query,
		p.UserID,
		p.Content,
		p.CreatedAt,
		p.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (r *MySQLPostRepository) UpdatePost(ctx context.Context, p *model.Post) error {

}
