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
		SELECT id, content, created_at, updated_at, user_id, parent_id
		FROM posts
		WHERE id = ?
	`
	row := r.DB.QueryRowContext(ctx, query, id)

	var p model.Post
	var createdAtUnix, updatedAtUnix int64
	var parentID sql.NullInt64
	if err := row.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID); err != nil {
		return nil, err
	}
	if parentID.Valid {
		p.ParentID = &parentID.Int64
	} else {
		p.ParentID = nil
	}
	p.CreatedAt = time.Unix(createdAtUnix, 0)
	p.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &p, nil
}

func (r *MySQLPostRepository) CreatePost(ctx context.Context, p *model.Post) (int64, error) {
	query := `
		INSERT INTO posts (content, created_at, updated_at, user_id, parent_id)
		VALUES (?, ?, ?, ?, ?)
	`

	var validParentID sql.NullInt64
	if p.ParentID != nil && *p.ParentID != 0 {
		validParentID = sql.NullInt64{Int64: *p.ParentID, Valid: true}
	} else {
		validParentID = sql.NullInt64{Valid: false}
	}

	result, err := r.DB.ExecContext(ctx, query,
		p.Content,
		p.CreatedAt.Unix(),
		p.UpdatedAt.Unix(),
		p.UserID,
		validParentID,
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
	p.UpdatedAt = time.Now()

	query := `
		UPDATE posts
		SET content = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.DB.ExecContext(ctx, query,
		p.Content,
		p.UpdatedAt.Unix(),
		p.ID,
	)
	return err
}

func (r *MySQLPostRepository) DeletePost(ctx context.Context, id int64) (bool, error) {
	query := `
		DELETE FROM posts
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

func (r *MySQLPostRepository) DeletePostsByUserID(ctx context.Context, userID int64) error {
	query := `DELETE FROM posts WHERE user_id = ?`
	_, err := r.DB.ExecContext(ctx, query, userID)
	return err
}

func (r *MySQLPostRepository) GetPostsByUserID(ctx context.Context, userID int64) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id
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
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		} else {
			p.ParentID = nil
		}
		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		posts = append(posts, &p)
	}

	return posts, nil
}

func (r *MySQLPostRepository) ListPosts(ctx context.Context) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id
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
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		} else {
			p.ParentID = nil
		}
		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		posts = append(posts, &p)
	}

	return posts, nil
}

func (r *MySQLPostRepository) SearchPosts(ctx context.Context, query string) ([]*model.Post, error) {
	searchQuery := `
		SELECT id, content, created_at, updated_at, user_id, parent_id
		FROM posts
		WHERE content LIKE ?
	`
	rows, err := r.DB.QueryContext(ctx, searchQuery, "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		} else {
			p.ParentID = nil
		}
		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		posts = append(posts, &p)
	}

	return posts, nil
}

func (r *MySQLPostRepository) GetRepliesByID(ctx context.Context, id int64) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id
		FROM posts
		WHERE parent_id = ?
	`
	rows, err := r.DB.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		} else {
			p.ParentID = nil
		}
		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		posts = append(posts, &p)
	}

	return posts, nil
}

func (r *MySQLPostRepository) ListTopLevelPosts(ctx context.Context) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id
		FROM posts
		WHERE parent_id IS NULL
	`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		} else {
			p.ParentID = nil
		}
		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		posts = append(posts, &p)
	}

	return posts, nil
}
