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

func (r *MySQLPostRepository) ListPosts(ctx context.Context, limit, offset int) ([]*model.Post, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM posts`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, content, created_at, updated_at, user_id, parent_id, deleted_at, reply_count
		FROM posts
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID, deletedAtUnix sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &deletedAtUnix, &p.ReplyCount); err != nil {
			return nil, 0, err
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

	return posts, total, nil
}

func (r *MySQLPostRepository) GetfollowersTopLevelPostsByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM favorite_users fu
		JOIN posts p ON fu.favorite_user_id = p.user_id
		WHERE fu.user_id = ? AND p.parent_id IS NULL AND p.deleted_at IS NULL
	`
	var countArgs []interface{}
	countArgs = append(countArgs, userID)
	countQuery, countArgs = AppendBlockFilter(ctx, countQuery, countArgs, "p.user_id")
	var total int
	if err := r.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT
		  p.id, p.content, p.created_at, p.updated_at, p.user_id, p.parent_id, p.reply_count, p.deleted_at,
		  (COUNT(f.id) * 2 + p.reply_count * 3 + 1)
		  / POW(((UNIX_TIMESTAMP() - p.created_at) / 3600.0) + 2, 1.5) AS score
		FROM favorite_users fu
		JOIN posts p ON fu.favorite_user_id = p.user_id
		LEFT JOIN favorites f ON f.post_id = p.id
		WHERE fu.user_id = ? AND p.parent_id IS NULL AND p.deleted_at IS NULL
	`
	var args []interface{}
	args = append(args, userID)
	query, args = AppendBlockFilter(ctx, query, args, "p.user_id")
	query += `
		GROUP BY p.id, p.content, p.created_at, p.updated_at, p.user_id, p.parent_id, p.reply_count, p.deleted_at
		ORDER BY score DESC, p.created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID, deletedAtUnix sql.NullInt64
		var score float64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount, &deletedAtUnix, &score); err != nil {
			return nil, 0, err
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

	return posts, total, rows.Err()
}

func (r *MySQLPostRepository) SearchPosts(ctx context.Context, query string) ([]*model.Post, error) {
	searchQuery := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count, deleted_at
		FROM posts
		WHERE content LIKE ?  AND deleted_at IS NULL
	`
	args := []interface{}{"%" + query + "%"}
	searchQuery, args = AppendBlockFilter(ctx, searchQuery, args, "user_id")
	searchQuery += " ORDER BY created_at DESC"

	rows, err := r.DB.QueryContext(ctx, searchQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID, deletedAtUnix sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount, &deletedAtUnix); err != nil {
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

func (r *MySQLPostRepository) ListTopLevelPosts(ctx context.Context, limit, offset int) ([]*model.Post, int, error) {
	countQuery := `SELECT COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL`
	var countArgs []interface{}
	countQuery, countArgs = AppendBlockFilter(ctx, countQuery, countArgs, "user_id")
	var total int
	if err := r.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
		FROM posts
		WHERE parent_id IS NULL AND deleted_at IS NULL
	`
	var args []interface{}
	query, args = AppendBlockFilter(ctx, query, args, "user_id")
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
			return nil, 0, err
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

	return posts, total, nil
}

// GetFeedPosts はハイブリッドスコアでソートされたトップレベル投稿を返す。
// score = (いいね数×2 + 返信数×3 + 1) / (経過時間h + 2)^1.5 × フォローブースト(1.5)
func (r *MySQLPostRepository) GetFeedPosts(ctx context.Context, viewerID int64, limit, offset int) ([]*model.Post, int, error) {
	countQuery := `SELECT COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL`
	var countArgs []interface{}
	countQuery, countArgs = AppendBlockFilter(ctx, countQuery, countArgs, "user_id")
	var total int
	if err := r.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	baseQuery := `
		SELECT
		  p.id, p.content, p.created_at, p.updated_at, p.user_id, p.parent_id, p.reply_count,
		  (COUNT(f.id) * 2 + p.reply_count * 3 + 1)
		  / POW(((UNIX_TIMESTAMP() - p.created_at) / 3600.0) + 2, 1.5)
		  * IF(fu.id IS NOT NULL, 1.5, 1.0) AS score
		FROM posts p
		LEFT JOIN favorites f ON f.post_id = p.id
		LEFT JOIN favorite_users fu ON fu.user_id = ? AND fu.favorite_user_id = p.user_id
		WHERE p.parent_id IS NULL AND p.deleted_at IS NULL
	`
	args := []interface{}{viewerID}
	baseQuery, args = AppendBlockFilter(ctx, baseQuery, args, "p.user_id")
	baseQuery += `
		GROUP BY p.id, p.content, p.created_at, p.updated_at, p.user_id, p.parent_id, p.reply_count, fu.id
		ORDER BY score DESC, p.created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)

	rows, err := r.DB.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		var score float64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount, &score); err != nil {
			return nil, 0, err
		}
		if parentID.Valid {
			p.ParentID = &parentID.Int64
		}
		p.CreatedAt = time.Unix(createdAtUnix, 0)
		p.UpdatedAt = time.Unix(updatedAtUnix, 0)
		posts = append(posts, &p)
	}
	return posts, total, rows.Err()
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
	// 1. リプライかどうかの確認と、現在の投稿者IDの取得
	var parentID sql.NullInt64
	var currentUserID int64
	err := r.DB.QueryRowContext(ctx, "SELECT parent_id, user_id FROM posts WHERE id = ?", postID).Scan(&parentID, &currentUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// 親がない（自身がトップレベル）場合はrootPostは存在しないのでnil
	if !parentID.Valid {
		return nil, nil
	}

	// 2. 祖先を遡って大元の投稿（rootPost）を取得するクエリ
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
	`

	// ⭕️ AppendBlockFilter を適用するための引数セットアップ
	args := []interface{}{postID, postID}

	// ancestors（CTE結果）の user_id に対してブロックフィルターをかける
	query, args = AppendBlockFilter(ctx, query, args, "user_id")

	// フィルター適用後に LIMIT を追加する
	query += " LIMIT 1"

	// 変更した query と args を使用して実行
	row := r.DB.QueryRowContext(ctx, query, args...)

	var p model.Post
	var createdAtUnix, updatedAtUnix int64
	var rootParentID, deletedAtUnix sql.NullInt64

	err = row.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &rootParentID, &deletedAtUnix, &p.ReplyCount)
	if err != nil {
		if err == sql.ErrNoRows {
			// ⭕️ ブロックフィルターに弾かれた、または物理削除された場合はダミーを生成
			now := time.Now()
			return &model.Post{
				ID:         0,
				Content:    "",
				CreatedAt:  now,
				UpdatedAt:  now,
				UserID:     currentUserID, // GraphQLエラー回避用
				DeletedAt:  &now,          // フロントエンドで「見つかりません」表示のトリガーとなる
				ReplyCount: 0,
			}, nil
		}
		return nil, err
	}

	if rootParentID.Valid {
		p.ParentID = &rootParentID.Int64
	}
	if deletedAtUnix.Valid {
		deletedAt := time.Unix(deletedAtUnix.Int64, 0)
		p.DeletedAt = &deletedAt
	}
	p.CreatedAt = time.Unix(createdAtUnix, 0)
	p.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &p, nil
}

func (r *MySQLPostRepository) GetFavoritePostsByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error) {
	countQuery := `
		SELECT COUNT(*) 
		FROM favorites f
		JOIN posts p ON f.post_id = p.id
		WHERE f.user_id = ? AND p.deleted_at IS NULL
	`
	var countArgs []interface{}
	countArgs = append(countArgs, userID)
	countQuery, countArgs = AppendBlockFilter(ctx, countQuery, countArgs, "p.user_id")

	var total int
	if err := r.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT p.id, p.content, p.created_at, p.updated_at, p.user_id, p.parent_id, p.reply_count, p.deleted_at
		FROM favorites f
		JOIN posts p ON f.post_id = p.id
		WHERE f.user_id = ? AND p.deleted_at IS NULL
	`
	var args []interface{}
	args = append(args, userID)
	query, args = AppendBlockFilter(ctx, query, args, "p.user_id")
	query, args = AppendBlockFilter(ctx, query, args, "f.user_id")
	query += " ORDER BY f.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID, deletedAtUnix sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount, &deletedAtUnix); err != nil {
			return nil, 0, err
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

	return posts, total, nil
}

func (r *MySQLPostRepository) GetPostsByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Post, int, error) {
	countQuery := `SELECT COUNT(*) FROM posts WHERE user_id = ? AND deleted_at IS NULL`
	countArgs := []interface{}{userID}
	countQuery, countArgs = AppendBlockFilter(ctx, countQuery, countArgs, "user_id")

	var total int
	if err := r.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, content, created_at, updated_at, user_id, parent_id, reply_count
		FROM posts
		WHERE user_id = ? AND deleted_at IS NULL
	`
	args := []interface{}{userID}
	query, args = AppendBlockFilter(ctx, query, args, "user_id")
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []*model.Post
	for rows.Next() {
		var p model.Post
		var createdAtUnix, updatedAtUnix int64
		var parentID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Content, &createdAtUnix, &updatedAtUnix, &p.UserID, &parentID, &p.ReplyCount); err != nil {
			return nil, 0, err
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

	return posts, total, nil
}
func (r *MySQLPostRepository) CountNewFeedPosts(ctx context.Context, viewerID int64, since time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM posts WHERE parent_id IS NULL AND deleted_at IS NULL AND created_at > ? AND user_id != ?`
	args := []interface{}{since.Unix(), viewerID}
	query, args = AppendBlockFilter(ctx, query, args, "user_id")
	var count int
	if err := r.DB.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
