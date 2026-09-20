package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLFavoriteUserRepository struct {
	DB *sql.DB
}

func NewMySQLFavoriteUserRepository(db *sql.DB) *MySQLFavoriteUserRepository {
	return &MySQLFavoriteUserRepository{DB: db}
}

func (r *MySQLFavoriteUserRepository) CreateFavoriteUser(ctx context.Context, fu *model.FavoriteUser) (int64, error) {
	now := time.Now()
	fu.CreatedAt = now
	query := `
		INSERT INTO favorite_users (user_id, favorite_user_id, created_at)
		VALUES (?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query,
		fu.UserID,
		fu.FavoriteUserID,
		fu.CreatedAt.Unix(),
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	fu.ID = id

	return id, nil
}

// DeleteFavoriteUser はお気に入り登録を外す。
// ブロック（usecase/block）が作成と同じ RunInTx の中から呼ぶので extractDB を通す。
func (r *MySQLFavoriteUserRepository) DeleteFavoriteUser(ctx context.Context, userID int64, favoriteUserID int64) (bool, error) {
	query := "DELETE FROM favorite_users WHERE user_id = ? AND favorite_user_id = ?"
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, query, userID, favoriteUserID)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (r *MySQLFavoriteUserRepository) ListFavoriteUsers(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.FavoriteUser, int, error) {
	total, err := countForPage(ctx, r.DB, q, `SELECT COUNT(*) FROM favorite_users WHERE user_id = ?`, userID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, user_id, favorite_user_id, created_at FROM favorite_users WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		userID, q.Limit, q.Offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var favoriteUsers []*model.FavoriteUser
	for rows.Next() {
		var fu model.FavoriteUser
		var createdAtUnix int64
		if err := rows.Scan(&fu.ID, &fu.UserID, &fu.FavoriteUserID, &createdAtUnix); err != nil {
			return nil, 0, err
		}
		fu.CreatedAt = time.Unix(createdAtUnix, 0)
		favoriteUsers = append(favoriteUsers, &fu)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return favoriteUsers, total, nil
}

func (r *MySQLFavoriteUserRepository) ListFollowers(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.FavoriteUser, int, error) {
	total, err := countForPage(ctx, r.DB, q, `SELECT COUNT(*) FROM favorite_users WHERE favorite_user_id = ?`, userID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, user_id, favorite_user_id, created_at FROM favorite_users WHERE favorite_user_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		userID, q.Limit, q.Offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var followers []*model.FavoriteUser
	for rows.Next() {
		var fu model.FavoriteUser
		var createdAtUnix int64
		if err := rows.Scan(&fu.ID, &fu.UserID, &fu.FavoriteUserID, &createdAtUnix); err != nil {
			return nil, 0, err
		}
		fu.CreatedAt = time.Unix(createdAtUnix, 0)
		followers = append(followers, &fu)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return followers, total, nil
}

func (r *MySQLFavoriteUserRepository) SearchFavoriteUsers(ctx context.Context, userID int64, keyword string, q repository.PageQuery) ([]*model.FavoriteUser, error) {
	// 並びを id 降順（新しい順）に固定してから窓を切る。ORDER BY 無しに LIMIT を
	// 足すとページごとに順序が変わりうるので、重複と抜けが出る。
	query := `
		SELECT fu.id, fu.user_id, fu.favorite_user_id, fu.created_at
		FROM favorite_users fu
		JOIN users u ON fu.favorite_user_id = u.id
		WHERE fu.user_id = ? AND (u.name LIKE ? OR u.account_id LIKE ?)
		ORDER BY fu.id DESC
		LIMIT ? OFFSET ?
	`

	searchParam := "%" + keyword + "%"
	rows, err := r.DB.QueryContext(ctx, query, userID, searchParam, searchParam, q.Limit, q.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favoriteUsers []*model.FavoriteUser
	for rows.Next() {
		var fu model.FavoriteUser
		var createdAtUnix int64
		if err := rows.Scan(
			&fu.ID,
			&fu.UserID,
			&fu.FavoriteUserID,
			&createdAtUnix,
		); err != nil {
			return nil, err
		}
		fu.CreatedAt = time.Unix(createdAtUnix, 0)
		favoriteUsers = append(favoriteUsers, &fu)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favoriteUsers, nil
}

func (r *MySQLFavoriteUserRepository) GetFavoriteUsersByUserID(ctx context.Context, userID int64, q repository.PageQuery) ([]*model.FavoriteUser, error) {
	// 並びの固定理由は SearchFavoriteUsers と同じ。
	query := "SELECT id, user_id, favorite_user_id, created_at FROM favorite_users WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := r.DB.QueryContext(ctx, query, userID, q.Limit, q.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favoriteUsers []*model.FavoriteUser
	for rows.Next() {
		var fu model.FavoriteUser
		var createdAtUnix int64
		if err := rows.Scan(
			&fu.ID,
			&fu.UserID,
			&fu.FavoriteUserID,
			&createdAtUnix,
		); err != nil {
			return nil, err
		}
		fu.CreatedAt = time.Unix(createdAtUnix, 0)
		favoriteUsers = append(favoriteUsers, &fu)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favoriteUsers, nil
}
