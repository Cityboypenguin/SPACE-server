package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLFavoriteRepository struct {
	DB *sql.DB
}

func NewMySQLFavoriteRepository(db *sql.DB) repository.FavoriteRepository {
	return &MySQLFavoriteRepository{DB: db}
}

func (r *MySQLFavoriteRepository) GetFavoriteByID(ctx context.Context, id int64) (*model.Favorite, error) {
	query := `SELECT id, post_id, user_id, created_at FROM favorites WHERE id = ?`
	row := r.DB.QueryRowContext(ctx, query, id)

	var favorite model.Favorite
	if err := row.Scan(&favorite.ID, &favorite.PostID, &favorite.UserID, &favorite.CreatedAt); err != nil {
		return nil, err
	}

	return &favorite, nil
}

func (r *MySQLFavoriteRepository) CreateFavorite(ctx context.Context, favorite *model.Favorite) (*model.Favorite, error) {
	now := time.Now()
	favorite.CreatedAt = now

	query := `
		INSERT INTO favorites (post_id, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.DB.ExecContext(ctx, query,
		favorite.PostID,
		favorite.UserID,
		favorite.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	favorite.ID = id

	return favorite, nil
}

func (r *MySQLFavoriteRepository) DeleteFavorite(ctx context.Context, id int64) error {
	query := `DELETE FROM favorites WHERE id = ?`
	_, err := r.DB.ExecContext(ctx, query, id)
	return err
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
		if err := rows.Scan(&favorite.ID, &favorite.UserID, &favorite.CreatedAt); err != nil {
			return nil, err
		}
		favorites = append(favorites, &favorite)
	}

	return favorites, nil
}
