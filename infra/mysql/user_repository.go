package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MySQLUserRepository struct {
	DB *sql.DB
}

func NewMySQLUserRepository(db *sql.DB) *MySQLUserRepository {
	return &MySQLUserRepository{DB: db}
}

func (r *MySQLUserRepository) SaveUser(ctx context.Context, u *model.User) error {
	now := time.Now().Unix()
	u.UpdatedAt = now

	if u.ID == 0 {
		u.CreatedAt = now
		id, err := r.CreateUser(ctx, u)
		if err != nil {
			return err
		}

		u.ID = id
	} else {
		if err := r.UpdateUser(ctx, u); err != nil {
			return err
		}
	}

	return nil
}

func (r *MySQLUserRepository) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	query := `
		SELECT id, user_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		WHERE id = ?
	`

	row := r.DB.QueryRowContext(ctx, query, id)

	var u model.User
	if err := row.Scan(
		&u.ID,
		&u.UserID,
		&u.Name,
		&u.Email,
		&u.HashedPassword,
		&u.Role,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &u, nil
}

func (r *MySQLUserRepository) DeleteUser(ctx context.Context, id int64) (bool, error) {
	query := `
		DELETE FROM users
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

func (r *MySQLUserRepository) ListUsers(ctx context.Context) ([]*model.User, error) {
	query := `
		SELECT id, user_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
	`

	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(
			&u.ID,
			&u.UserID,
			&u.Name,
			&u.Email,
			&u.HashedPassword,
			&u.Role,
			&u.Status,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}

	return users, nil
}

func (r *MySQLUserRepository) SearchUsersByName(ctx context.Context, name string) ([]*model.User, error) {
	query := `
		SELECT id, user_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		WHERE name LIKE ?
	`

	rows, err := r.DB.QueryContext(ctx, query, "%"+name+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(
			&u.ID,
			&u.UserID,
			&u.Name,
			&u.Email,
			&u.HashedPassword,
			&u.Role,
			&u.Status,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}

	return users, nil
}

func (r *MySQLUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, user_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		WHERE email = ?
		LIMIT 1
	`

	row := r.DB.QueryRowContext(ctx, query, email)

	var u model.User
	if err := row.Scan(
		&u.ID,
		&u.UserID,
		&u.Name,
		&u.Email,
		&u.HashedPassword,
		&u.Role,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &u, nil
}

func (r *MySQLUserRepository) UpdateUser(ctx context.Context, u *model.User) error {
	query := `
		UPDATE users
		SET name = ?, email = ?, hashed_password = ?, role = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.DB.ExecContext(ctx, query,
		u.Name,
		u.Email,
		u.HashedPassword,
		u.Role,
		u.Status,
		u.UpdatedAt,
		u.ID,
	)
	return err
}

func (r *MySQLUserRepository) CreateUser(ctx context.Context, u *model.User) (int64, error) {
	query := `
		INSERT INTO users (user_id, name, email, hashed_password, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.DB.ExecContext(ctx, query,
		u.UserID,
		u.Name,
		u.Email,
		u.HashedPassword,
		u.Role,
		u.Status,
		u.CreatedAt,
		u.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
