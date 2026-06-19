package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
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

func (r *MySQLFavoriteUserRepository) DeleteFavoriteUser(ctx context.Context, userID int64, favoriteUserID int64) (bool, error) {
	query := "DELETE FROM favorite_users WHERE user_id = ? AND favorite_user_id = ?"
	result, err := r.DB.ExecContext(ctx, query, userID, favoriteUserID)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (r *MySQLFavoriteUserRepository) ListFavoriteUsers(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM favorite_users WHERE user_id = ?`, userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, user_id, favorite_user_id, created_at FROM favorite_users WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
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

func (r *MySQLFavoriteUserRepository) ListFollowers(ctx context.Context, userID int64, limit, offset int) ([]*model.FavoriteUser, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM favorite_users WHERE favorite_user_id = ?`, userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, user_id, favorite_user_id, created_at FROM favorite_users WHERE favorite_user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
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

func (r *MySQLFavoriteUserRepository) SearchFavoriteUsers(ctx context.Context, userID int64, keyword string) ([]*model.FavoriteUser, error) {
	query := `
		SELECT fu.id, fu.user_id, fu.favorite_user_id, fu.created_at
		FROM favorite_users fu
		JOIN users u ON fu.favorite_user_id = u.id
		WHERE fu.user_id = ? AND (u.name LIKE ? OR u.account_id LIKE ?)
	`

	searchParam := "%" + keyword + "%"
	rows, err := r.DB.QueryContext(ctx, query, userID, searchParam, searchParam)
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

func (r *MySQLFavoriteUserRepository) GetFavoriteUsersByUserID(ctx context.Context, userID int64) ([]*model.FavoriteUser, error) {
	query := "SELECT id, user_id, favorite_user_id, created_at FROM favorite_users WHERE user_id = ?"
	rows, err := r.DB.QueryContext(ctx, query, userID)
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
