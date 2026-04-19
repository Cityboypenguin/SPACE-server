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
	if err := row.Scan(&p.ID, &p.User, &p.Content, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
		p.User,
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
	query := `
		UPDATE posts
		SET user_id = ?, title = ?, content = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.DB.ExecContext(ctx, query,
		p.User,
		p.Content,
		p.UpdatedAt,
		p.ID,
	)
	return err
}

func (r *MySQLPostRepository) DeletePost(ctx context.Context, id int64) error {
	query := `
		DELETE FROM posts
		WHERE id = ?
	`
	_, err := r.DB.ExecContext(ctx, query, id)
	return err
}

func (r *MySQLPostRepository) GetPostsByUserID(ctx context.Context, userID string) ([]*model.Post, error) {
	query := `
		SELECT id, user_id, title, content, created_at, updated_at
		FROM posts
		WHERE user_id = ?
	`
	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		if err := rows.Scan(&p.ID, &p.User, &p.Content, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		posts = append(posts, &p)
	}

	return posts, nil
}

func (r *MySQLPostRepository) ListPosts(ctx context.Context) ([]*model.Post, error) {
	query := `
		SELECT id, user_id, title, content, created_at, updated_at
		FROM posts
	`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		if err := rows.Scan(&p.ID, &p.User, &p.Content, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		posts = append(posts, &p)
	}

	return posts, nil
}
