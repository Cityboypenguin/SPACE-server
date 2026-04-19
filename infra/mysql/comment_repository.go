package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLCommentRepository struct {
	DB *sql.DB
}

func NewMySQLCommentRepository(db *sql.DB) repository.CommentRepository {
	return &MySQLCommentRepository{DB: db}
}

func (r *MySQLCommentRepository) GetCommentByID(ctx context.Context, id int64) (*model.Comment, error) {
	query := `
		SELECT id, post_id, user_id, content, created_at, updated_at
		FROM comments
		WHERE id = ?
	`
	row := r.DB.QueryRowContext(ctx, query, id)

	var c model.Comment
	if err := row.Scan(&c.ID, &c.Post, &c.User, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}

	return &c, nil
}

func (r *MySQLCommentRepository) SaveComment(ctx context.Context, c *model.Comment) error {
	now := time.Now()
	c.UpdatedAt = now

	if c.ID == 0 {
		c.CreatedAt = now
		id, err := r.CreateComment(ctx, c)
		if err != nil {
			return err
		}

		c.ID = id
	} else {
		if err := r.UpdateComment(ctx, c); err != nil {
			return err
		}
	}

	return nil
}

func (r *MySQLCommentRepository) CreateComment(ctx context.Context, c *model.Comment) (int64, error) {
	query := `
		INSERT INTO comments (post_id, user_id, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query, c.Post, c.User, c.Content, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *MySQLCommentRepository) GetCommentsByPostID(ctx context.Context, postID int64) ([]*model.Comment, error) {
	query := `
		SELECT id, post_id, user_id, content, created_at, updated_at
		FROM comments
		WHERE post_id = ?
	`
	rows, err := r.DB.QueryContext(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*model.Comment
	for rows.Next() {
		var c model.Comment
		if err := rows.Scan(&c.ID, &c.Post, &c.User, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, &c)
	}

	return comments, nil
}

func (r *MySQLCommentRepository) DeleteComment(ctx context.Context, id int64) (bool, error) {
	query := `
		DELETE FROM comments
		WHERE id = ?
	`
	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *MySQLCommentRepository) UpdateComment(ctx context.Context, c *model.Comment) error {
	query := `
		UPDATE comments
		SET post_id = ?, user_id = ?, content = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.DB.ExecContext(ctx, query, c.Post, c.User, c.Content, c.UpdatedAt, c.ID)
	return err
}
