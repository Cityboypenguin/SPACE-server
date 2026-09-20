package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MySQLFavoriteRepository struct {
	DB *sql.DB
}

func NewMySQLFavoriteRepository(db *sql.DB) *MySQLFavoriteRepository {
	return &MySQLFavoriteRepository{DB: db}
}

func (r *MySQLFavoriteRepository) GetFavoriteByID(ctx context.Context, id int64) (*model.Favorite, error) {
	query := `SELECT id, post_id, user_id, created_at FROM favorites WHERE id = ?`
	row := r.DB.QueryRowContext(ctx, query, id)

	var favorite model.Favorite
	var createdAtUnix int64

	if err := row.Scan(&favorite.ID, &favorite.PostID, &favorite.UserID, &createdAtUnix); err != nil {
		return nil, err
	}

	return &favorite, nil
}

func (r *MySQLFavoriteRepository) CreateFavorite(ctx context.Context, f *model.Favorite) (int64, error) {
	now := time.Now()
	f.CreatedAt = now
	query := `
		INSERT INTO favorites (post_id, user_id, created_at) 
		VALUES (?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query,
		f.PostID,
		f.UserID,
		f.CreatedAt.Unix(),
	)
	if err != nil {
		// favorites には UNIQUE KEY unique_user_post (user_id, post_id) があるので、
		// 二重登録はここで 1062 として返る。呼び出し側が事前 SELECT せずに済むよう
		// repository.ErrDuplicateKey に包んで返す。
		return 0, wrapDuplicateKey(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	f.ID = id

	return id, nil
}

func (r *MySQLFavoriteRepository) DeleteFavorite(ctx context.Context, id int64) (bool, error) {
	query := `DELETE FROM favorites WHERE id = ?`
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

func (r *MySQLFavoriteRepository) GetFavoritesByPostID(ctx context.Context, postID int64) ([]*model.Favorite, error) {
	query := `SELECT id, user_id, created_at FROM favorites WHERE post_id = ?`
	rows, err := r.DB.QueryContext(ctx, query, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []*model.Favorite
	for rows.Next() {
		var favorite model.Favorite
		favorite.PostID = postID
		var createdAtUnix int64
		var userID int64
		if err := rows.Scan(&favorite.ID, &userID, &createdAtUnix); err != nil {
			return nil, err
		}
		favorite.UserID = userID
		favorite.CreatedAt = time.Unix(createdAtUnix, 0)
		favorites = append(favorites, &favorite)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favorites, nil
}

func (r *MySQLFavoriteRepository) GetFavoritesByUserID(ctx context.Context, userID int64) ([]*model.Favorite, error) {
	query := `SELECT id, post_id, created_at FROM favorites WHERE user_id = ?`
	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []*model.Favorite
	for rows.Next() {
		var favorite model.Favorite
		var createdAtUnix int64
		favorite.UserID = userID
		if err := rows.Scan(&favorite.ID, &favorite.PostID, &createdAtUnix); err != nil {
			return nil, err
		}
		favorite.CreatedAt = time.Unix(createdAtUnix, 0)
		favorites = append(favorites, &favorite)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favorites, nil
}

func (r *MySQLFavoriteRepository) GetFavoriteByUserIDAndPostID(ctx context.Context, userID int64, postID int64) (*model.Favorite, error) {
	query := `SELECT id, created_at FROM favorites WHERE user_id = ? AND post_id = ?`
	row := r.DB.QueryRowContext(ctx, query, userID, postID)

	var favorite model.Favorite
	var createdAtUnix int64

	favorite.UserID = userID
	favorite.PostID = postID

	if err := row.Scan(&favorite.ID, &createdAtUnix); err != nil {
		return nil, err
	}

	favorite.CreatedAt = time.Unix(createdAtUnix, 0)

	return &favorite, nil
}

func (r *MySQLFavoriteRepository) DeleteFavoriteByUserIDAndPostID(ctx context.Context, userID int64, postID int64) (bool, error) {
	query := `DELETE FROM favorites WHERE user_id = ? AND post_id = ?`
	result, err := r.DB.ExecContext(ctx, query, userID, postID)
	if err != nil {
		return false, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

// GetFavoritesByPostIDs は複数のPostIDに紐づくいいねを1回のSQLで取得する
func (r *MySQLFavoriteRepository) GetFavoritesByPostIDs(ctx context.Context, postIDs []int64) (map[int64][]*model.Favorite, error) {
	if len(postIDs) == 0 {
		return make(map[int64][]*model.Favorite), nil
	}

	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, len(postIDs))
	for i, id := range postIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, post_id, created_at
		FROM favorites
		WHERE post_id IN (%s)
		ORDER BY created_at DESC, id DESC
	`, strings.Join(placeholders, ","))

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64][]*model.Favorite)
	for rows.Next() {
		var f model.Favorite
		var createdAtUnix int64
		// ※DBのカラム構成に合わせてScanする
		if err := rows.Scan(&f.ID, &f.UserID, &f.PostID, &createdAtUnix); err != nil {
			return nil, err
		}
		f.CreatedAt = time.Unix(createdAtUnix, 0)
		result[f.PostID] = append(result[f.PostID], &f)
	}
	return result, rows.Err()
}

// CountFavoritesByPostIDs は投稿ごとのいいね件数を1クエリで数える。
//
// GetFavoritesByPostIDs と違って行は1つも持ち帰らない。表示に要るのが件数だけの
// 経路（一覧・詳細の LikeButton）はこちらを通す。
func (r *MySQLFavoriteRepository) CountFavoritesByPostIDs(ctx context.Context, postIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, len(postIDs))
	for i, id := range postIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT post_id, COUNT(*)
		FROM favorites
		WHERE post_id IN (%s)
		GROUP BY post_id
	`, strings.Join(placeholders, ","))

	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var postID int64
		var count int
		if err := rows.Scan(&postID, &count); err != nil {
			return nil, err
		}
		result[postID] = count
	}
	return result, rows.Err()
}

// ListPostIDsFavoritedBy は postIDs のうち userID がいいねしたものを返す。
//
// 「自分がいいねしたか」を出すためだけに全いいね行を運ぶのをやめるための口。
// 自分の行しか見ないので、投稿のいいね数がいくつでも戻る行数は
// 「一覧に出ている投稿のうち自分がいいねした数」で頭打ちになる。
func (r *MySQLFavoriteRepository) ListPostIDsFavoritedBy(ctx context.Context, userID int64, postIDs []int64) (map[int64]bool, error) {
	result := make(map[int64]bool, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, 0, len(postIDs)+1)
	args = append(args, userID)
	for i, id := range postIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT post_id
		FROM favorites
		WHERE user_id = ? AND post_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := extractDB(ctx, r.DB).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var postID int64
		if err := rows.Scan(&postID); err != nil {
			return nil, err
		}
		result[postID] = true
	}
	return result, rows.Err()
}
