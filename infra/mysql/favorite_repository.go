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

func (r *MySQLFavoriteRepository) ListFavorites(ctx context.Context) ([]*model.Favorite, error) {
	query := `SELECT id, post_id, user_id, created_at FROM favorites`
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []*model.Favorite
	for rows.Next() {
		var favorite model.Favorite
		var createdAtUnix int64

		favorite.Post = &model.Post{}
		favorite.User = &model.User{}

		if err := rows.Scan(&favorite.ID, &favorite.Post.ID, &favorite.User.ID, &createdAtUnix); err != nil {
			return nil, err
		}
		favorite.CreatedAt = time.Unix(createdAtUnix, 0)
		favorites = append(favorites, &favorite)
	}

	return favorites, nil
}

func (r *MySQLFavoriteRepository) GetFavoriteByID(ctx context.Context, id int64) (*model.Favorite, error) {
	query := `SELECT id, post_id, user_id, created_at FROM favorites WHERE id = ?`
	row := r.DB.QueryRowContext(ctx, query, id)

	var favorite model.Favorite
	var createdAtUnix int64

	favorite.Post = &model.Post{}
	favorite.User = &model.User{}

	if err := row.Scan(&favorite.ID, &favorite.Post.ID, &favorite.User.ID, &createdAtUnix); err != nil {
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
		f.Post.ID,
		f.User.ID,
		f.CreatedAt.Unix(),
	)
	if err != nil {
		return 0, err
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
		favorite.Post = &model.Post{ID: postID}
		if err := rows.Scan(&favorite.ID, &favorite.User, &favorite.CreatedAt); err != nil {
			return nil, err
		}
		favorites = append(favorites, &favorite)
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
		favorite.User = &model.User{ID: userID}
		if err := rows.Scan(&favorite.ID, &favorite.Post, &favorite.CreatedAt); err != nil {
			return nil, err
		}
		favorites = append(favorites, &favorite)
	}

	return favorites, nil
}

func (r *MySQLFavoriteRepository) GetFavoriteByUserIDAndPostID(ctx context.Context, userID int64, postID int64) (*model.Favorite, error) {
	query := `SELECT id, created_at FROM favorites WHERE user_id = ? AND post_id = ?`
	row := r.DB.QueryRowContext(ctx, query, userID, postID)

	var favorite model.Favorite
	var createdAtUnix int64

	favorite.User = &model.User{ID: userID}
	favorite.Post = &model.Post{ID: postID}

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
