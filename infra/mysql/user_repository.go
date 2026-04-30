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
	now := time.Now()
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
		SELECT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		WHERE id = ?
	`

	row := r.DB.QueryRowContext(ctx, query, id)

	var u model.User
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(
		&u.ID,
		&u.AccountID,
		&u.Name,
		&u.Email,
		&u.HashedPassword,
		&u.Role,
		&u.Status,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	u.CreatedAt = time.Unix(createdAtUnix, 0)
	u.UpdatedAt = time.Unix(updatedAtUnix, 0)

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
		SELECT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
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
		var createdAtUnix, updatedAtUnix int64
		if err := rows.Scan(
			&u.ID,
			&u.AccountID,
			&u.Name,
			&u.Email,
			&u.HashedPassword,
			&u.Role,
			&u.Status,
			&createdAtUnix,
			&updatedAtUnix,
		); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(createdAtUnix, 0)
		u.UpdatedAt = time.Unix(updatedAtUnix, 0)
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (r *MySQLUserRepository) SearchUsersByName(ctx context.Context, name string) ([]*model.User, error) {
	query := `
		SELECT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
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
		var createdAtUnix, updatedAtUnix int64
		if err := rows.Scan(
			&u.ID,
			&u.AccountID,
			&u.Name,
			&u.Email,
			&u.HashedPassword,
			&u.Role,
			&u.Status,
			&createdAtUnix,
			&updatedAtUnix,
		); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(createdAtUnix, 0)
		u.UpdatedAt = time.Unix(updatedAtUnix, 0)
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (r *MySQLUserRepository) FindByAccountID(ctx context.Context, accountID string) (*model.User, error) {
	query := `
		SELECT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		WHERE account_id = ?
		LIMIT 1
	`

	row := r.DB.QueryRowContext(ctx, query, accountID)

	var u model.User
	if err := row.Scan(
		&u.ID,
		&u.AccountID,
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

func (r *MySQLUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		WHERE email = ?
		LIMIT 1
	`
	row := r.DB.QueryRowContext(ctx, query, email)

	var u model.User
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(
		&u.ID,
		&u.AccountID,
		&u.Name,
		&u.Email,
		&u.HashedPassword,
		&u.Role,
		&u.Status,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	u.CreatedAt = time.Unix(createdAtUnix, 0)
	u.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &u, nil
}

func (r *MySQLUserRepository) UpdateUser(ctx context.Context, u *model.User) error {
	u.UpdatedAt = time.Now()

	query := `
		UPDATE users
		SET user_id = ?, name = ?, email = ?, hashed_password = ?, role = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.DB.ExecContext(ctx, query,
		u.AccountID,
		u.Name,
		u.Email,
		u.HashedPassword,
		u.Role,
		u.Status,
		u.UpdatedAt.Unix(),
		u.ID,
	)
	return err
}

func (r *MySQLUserRepository) CreateUser(ctx context.Context, u *model.User) (int64, error) {
	query := `
		INSERT INTO users (account_id, name, email, hashed_password, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.DB.ExecContext(ctx, query,
		u.AccountID,
		u.Name,
		u.Email,
		u.HashedPassword,
		u.Role,
		u.Status,
		u.CreatedAt.Unix(),
		u.UpdatedAt.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
