package mysql

import (
	"context"
	"database/sql"
	"log"
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
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM posts
		WHERE id = ? AND deleted_at IS NULL
	`
	row := r.DB.QueryRowContext(ctx, query, id)

	var p model.Post
	var createdAtUnix, updatedAtUnix int64
	var parentID, deletedAtUnix sql.NullInt64
	if err := row.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &deletedAtUnix, &p.ReplyCount); err != nil {
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

	// 🚨修正箇所： r.DB ではなく tx を使う！
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
		// ▼ 監視カメラ：ループがどこまで遡っているかを出力する
		log.Printf("[DEBUG] ID: %d の reply_count を +1 します\n", currentParentID.Int64)

		updateQuery := `
			UPDATE posts 
			SET reply_count = reply_count + 1 
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, updateQuery, currentParentID.Int64); err != nil {
			log.Printf("[ERROR] UPDATE失敗: %v\n", err)
			return 0, err
		}

		var nextParentID sql.NullInt64
		getParentQuery := `SELECT parent_id FROM posts WHERE id = ?`
		err := tx.QueryRowContext(ctx, getParentQuery, currentParentID.Int64).Scan(&nextParentID)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("[DEBUG] ID: %d の親は見つかりませんでした（一番上に到達）\n", currentParentID.Int64)
				break
			}
			log.Printf("[ERROR] 親の検索に失敗: %v\n", err)
			return 0, err
		}
		log.Printf("[DEBUG] 次の親IDは見つかりましたか？: %+v\n", nextParentID)
		currentParentID = nextParentID
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] コミット失敗: %v\n", err)
		return 0, err
	}

	log.Println("[DEBUG] CreatePost 正常完了！")
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
		WHERE id = ? AND deleted_at IS NULL
	`

	result, err := r.DB.ExecContext(ctx, deleteQuery, time.Now().Unix(), time.Now().Unix(), id)
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
	query := `DELETE FROM posts WHERE user_id = ?`
	_, err := r.DB.ExecContext(ctx, query, userID)
	return err
}

func (r *MySQLPostRepository) GetPostsByUserID(ctx context.Context, userID int64) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
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

func (r *MySQLPostRepository) ListPosts(ctx context.Context) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
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
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
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

func (r *MySQLPostRepository) GetRepliesByID(ctx context.Context, id int64) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM posts
		WHERE parent_id = ? AND deleted_at IS NULL
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

func (r *MySQLPostRepository) ListTopLevelPosts(ctx context.Context) ([]*model.Post, error) {
	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM posts
		WHERE parent_id IS NULL AND deleted_at IS NULL
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
