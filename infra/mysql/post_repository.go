package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
		FROM posts
		WHERE id = ? AND deleted_at IS NULL
	`
	args := []interface{}{id}

	query, args = AppendBlockFilter(ctx, query, args, "user_id")
	row := r.DB.QueryRowContext(ctx, query, args...)

	var p model.Post
	var createdAtUnix, updatedAtUnix int64
	var parentID sql.NullInt64
	if err := row.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
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

func (r *MySQLPostRepository) GetPostByIDIncludeDeleted(ctx context.Context, id int64) (*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM posts
		WHERE id = ?
	`
	row := r.DB.QueryRowContext(ctx, query, id)

	var p model.Post
	var createdAtUnix, updatedAtUnix int64
	var parentID, deletedAtUnix sql.NullInt64
	if err := row.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &deletedAtUnix, &p.ReplyCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if parentID.Valid {
		p.ParentID = &parentID.Int64
	} else {
		p.ParentID = nil
	}

	if deletedAtUnix.Valid {
		deletedAt := time.Unix(deletedAtUnix.Int64, 0)
		p.DeletedAt = &deletedAt
	} else {
		p.DeletedAt = nil
	}

	p.CreatedAt = time.Unix(createdAtUnix, 0)
	p.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &p, nil
}

func (r *MySQLPostRepository) CreatePost(ctx context.Context, p *model.Post) (int64, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	insertQuery := `
		INSERT INTO posts (content, created_at, updated_at, user_id, parent_id)
		VALUES (?, ?, ?, ?, ?)
	`

	var validParentID sql.NullInt64
	if p.ParentID != nil && *p.ParentID != 0 {
		validParentID = sql.NullInt64{Int64: *p.ParentID, Valid: true}
	} else {
		validParentID = sql.NullInt64{Valid: false}
	}

	result, err := tx.ExecContext(ctx, insertQuery,
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

	currentParentID := validParentID
	for currentParentID.Valid {

		updateQuery := `
			UPDATE posts 
			SET reply_count = reply_count + 1 
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, updateQuery, currentParentID.Int64); err != nil {
			return 0, err
		}

		var nextParentID sql.NullInt64
		getParentQuery := `SELECT parent_id FROM posts WHERE id = ?`
		err := tx.QueryRowContext(ctx, getParentQuery, currentParentID.Int64).Scan(&nextParentID)
		if err != nil {
			if err == sql.ErrNoRows {
				break
			}
			return 0, err
		}
		currentParentID = nextParentID
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return id, nil
}

func (r *MySQLPostRepository) UpdatePost(ctx context.Context, p *model.Post) error {
	query := `
		UPDATE posts 
		SET content = ?, updated_at = ? 
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL
	`
	res, err := r.DB.ExecContext(ctx, query, p.Content, p.UpdatedAt.Unix(), p.ID, p.UserID)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	return nil
}

func (r *MySQLPostRepository) DeletePost(ctx context.Context, id int64) (bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var parentID sql.NullInt64
	getParentQuery := `SELECT parent_id FROM posts WHERE id = ? AND deleted_at IS NULL`
	err = tx.QueryRowContext(ctx, getParentQuery, id).Scan(&parentID)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	var currentReplyCount int64
	getReplyCountQuery := `SELECT reply_count FROM posts WHERE id = ? AND deleted_at IS NULL`
	err = tx.QueryRowContext(ctx, getReplyCountQuery, id).Scan(&currentReplyCount)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	deleteQuery := `
		UPDATE posts
		SET deleted_at = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now().Unix()
	result, err := tx.ExecContext(ctx, deleteQuery, now, now, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	currentParentID := parentID
	for affected > 0 && currentParentID.Valid {
		updateQuery := `
			UPDATE posts 
			SET reply_count = GREATEST(0, reply_count - ?) 
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, updateQuery, currentReplyCount+1, currentParentID.Int64); err != nil {
			return false, err
		}

		var nextParentID sql.NullInt64
		getParentQuery := `SELECT parent_id FROM posts WHERE id = ?`
		err := tx.QueryRowContext(ctx, getParentQuery, currentParentID.Int64).Scan(&nextParentID)
		if err != nil {
			if err == sql.ErrNoRows {
				break
			}
			return false, err
		}
		currentParentID = nextParentID
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *MySQLPostRepository) DeletePostsByUserID(ctx context.Context, userID int64) error {
	query := `
		UPDATE posts
		SET deleted_at = ?, updated_at = ?
		WHERE user_id = ? AND deleted_at IS NULL
	`
	now := time.Now().Unix()
	_, err := r.DB.ExecContext(ctx, query, now, now, userID)
	return err
}

func (r *MySQLPostRepository) GetPostsByUserID(ctx context.Context, userID int64) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
		FROM posts
		WHERE user_id = ? AND deleted_at IS NULL
	`

	args := []interface{}{userID}

	query, args = AppendBlockFilter(ctx, query, args, "user_id")

	query += " ORDER BY created_at DESC"

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
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
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM posts
		ORDER BY created_at DESC
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
		var parentID, deletedAtUnix sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &deletedAtUnix, &p.ReplyCount); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		} else {
			p.ParentID = nil
		}

		if deletedAtUnix.Valid {
			deletedAt := time.Unix(deletedAtUnix.Int64, 0)
			p.DeletedAt = &deletedAt
		} else {
			p.DeletedAt = nil
		}

		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		posts = append(posts, &p)
	}

	return posts, nil
}

func (r *MySQLPostRepository) SearchPosts(ctx context.Context, query string) ([]*model.Post, error) {
	searchQuery := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
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
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
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
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
		FROM posts
		WHERE parent_id = ? AND deleted_at IS NULL
	`

	args := []interface{}{id}
	query, args = AppendBlockFilter(ctx, query, args, "user_id")
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
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
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
		FROM posts
		WHERE parent_id IS NULL AND deleted_at IS NULL
	`

	var args []interface{}
	query, args = AppendBlockFilter(ctx, query, args, "user_id")

	query += " ORDER BY created_at DESC"

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
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

// GetRepliesByPostIDs は複数の親PostIDに紐づく返信を1回のSQLで取得する
func (r *MySQLPostRepository) GetRepliesByPostIDs(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error) {
	if len(parentIDs) == 0 {
		return make(map[int64][]*model.Post), nil
	}

	placeholders := make([]string, len(parentIDs))
	args := make([]interface{}, len(parentIDs))
	for i, id := range parentIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
		FROM posts
		WHERE parent_id IN (%s) AND deleted_at IS NULL
	`, strings.Join(placeholders, ","))

	query, args = AppendBlockFilter(ctx, query, args, "user_id")
	query += " ORDER BY created_at ASC"

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64][]*model.Post)
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		} else {
			p.ParentID = nil
		}

		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)

		if p.ParentID != nil {
			result[*p.ParentID] = append(result[*p.ParentID], &p)
		}
	}
	return result, rows.Err()
}

func (r *MySQLPostRepository) GetRepliesByPostIDsIncludeDeleted(ctx context.Context, parentIDs []int64) (map[int64][]*model.Post, error) {
	if len(parentIDs) == 0 {
		return make(map[int64][]*model.Post), nil
	}

	placeholders := make([]string, len(parentIDs))
	args := make([]interface{}, len(parentIDs))
	for i, id := range parentIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	// ⭕️ AND deleted_at IS NULL を除外
	query := fmt.Sprintf(`
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM posts
		WHERE parent_id IN (%s)
	`, strings.Join(placeholders, ","))

	query, args = AppendBlockFilter(ctx, query, args, "user_id")
	query += " ORDER BY created_at ASC"

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64][]*model.Post)
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID, deletedAtUnix sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &deletedAtUnix, &p.ReplyCount); err != nil {
			return nil, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		}
		if deletedAtUnix.Valid {
			deletedAt := time.Unix(deletedAtUnix.Int64, 0)
			p.DeletedAt = &deletedAt
		}

		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)

		if p.ParentID != nil {
			result[*p.ParentID] = append(result[*p.ParentID], &p)
		}
	}
	return result, rows.Err()
}

// infra/mysql/post_repository.go

func (r *MySQLPostRepository) GetRootPost(ctx context.Context, postID int64) (*model.Post, error) {
	query := `
		WITH RECURSIVE ancestors AS (
			SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
			FROM posts
			WHERE id = ?
			
			UNION ALL
			
			SELECT p.id, p.content, p.created_at, p.updated_at, p.user_id, p.parent_id, p.deleted_at, p.reply_count
			FROM posts p
			INNER JOIN ancestors a ON a.parent_id = p.id
		)
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM ancestors
		WHERE parent_id IS NULL AND id != ?
		LIMIT 1
	`

	row := r.DB.QueryRowContext(ctx, query, postID, postID)

	var p model.Post
	var createdAtUnix, updatedAtUnix int64
	var parentID, deletedAtUnix sql.NullInt64
	if err := row.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &deletedAtUnix, &p.ReplyCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if parentID.Valid {
		p.ParentID = &parentID.Int64
	}
	if deletedAtUnix.Valid {
		deletedAt := time.Unix(deletedAtUnix.Int64, 0)
		p.DeletedAt = &deletedAt
	}
	p.CreatedAt = time.Unix(createdAtUnix, 0)
	p.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &p, nil
}
