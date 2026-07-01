package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	drivermysql "github.com/go-sql-driver/mysql"
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

	row := extractDB(ctx, r.DB).QueryRowContext(ctx, query, id)

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

func (r *MySQLUserRepository) GetUsersByIDs(ctx context.Context, ids []int64) ([]*model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(`
		SELECT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users WHERE id IN (%s)`, placeholders)

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var u model.User
		var createdAtUnix, updatedAtUnix int64
		if err := rows.Scan(
			&u.ID, &u.AccountID, &u.Name, &u.Email, &u.HashedPassword,
			&u.Role, &u.Status, &createdAtUnix, &updatedAtUnix,
		); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(createdAtUnix, 0)
		u.UpdatedAt = time.Unix(updatedAtUnix, 0)
		users = append(users, &u)
	}
	return users, nil
}

func (r *MySQLUserRepository) DeleteUser(ctx context.Context, id int64) (bool, error) {
	query := `DELETE FROM users WHERE id = ?`
	result, err := extractDB(ctx, r.DB).ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *MySQLUserRepository) ListUsers(ctx context.Context, limit, offset int) ([]*model.User, int, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		u.CreatedAt = time.Unix(createdAtUnix, 0)
		u.UpdatedAt = time.Unix(updatedAtUnix, 0)
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *MySQLUserRepository) SearchUsersByKeyword(ctx context.Context, keyword string, limit, offset int) ([]*model.User, int, error) {
	searchParam := "%" + keyword + "%"

	var total int
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT id) FROM users WHERE name LIKE ? OR account_id LIKE ?`,
		searchParam, searchParam,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.DB.QueryContext(ctx, `
		SELECT DISTINCT id, account_id, name, email, hashed_password, role, status, created_at, updated_at
		FROM users
		WHERE name LIKE ? OR account_id LIKE ?
		ORDER BY name ASC, id ASC
		LIMIT ? OFFSET ?
	`, searchParam, searchParam, limit, offset)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		u.CreatedAt = time.Unix(createdAtUnix, 0)
		u.UpdatedAt = time.Unix(updatedAtUnix, 0)
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
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
		SET account_id = ?, name = ?, email = ?, hashed_password = ?, role = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := extractDB(ctx, r.DB).ExecContext(ctx, query,
		u.AccountID,
		u.Name,
		u.Email,
		u.HashedPassword,
		u.Role,
		u.Status,
		u.UpdatedAt.Unix(),
		u.ID,
	)
	if err != nil {
		if mysqlErr, ok := err.(*drivermysql.MySQLError); ok && mysqlErr.Number == 1062 {
			if strings.Contains(mysqlErr.Message, "account_id") {
				return errors.New("account_id is already taken")
			}
			return errors.New("email update failed")
		}
		return err
	}
	return nil
}

func (r *MySQLUserRepository) CreateUser(ctx context.Context, u *model.User) (int64, error) {
	query := `
		INSERT INTO users (account_id, name, email, hashed_password, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := extractDB(ctx, r.DB).ExecContext(ctx, query,
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
		if mysqlErr, ok := err.(*drivermysql.MySQLError); ok && mysqlErr.Number == 1062 {
			if strings.Contains(mysqlErr.Message, "account_id") {
				return 0, errors.New("account_id is already taken")
			}
			// メールアドレス重複は詳細を開示しない（ユーザー列挙攻撃対策）
			return 0, errors.New("registration failed: check your input")
		}
		return 0, err
	}
	return result.LastInsertId()
}

func (r *MySQLUserRepository) UpdateLastActiveAt(ctx context.Context, userID int64, now int64) error {
	_, err := extractDB(ctx, r.DB).ExecContext(ctx,
		`UPDATE users SET last_active_at = ? WHERE id = ? AND (last_active_at IS NULL OR last_active_at < ?)`,
		now, userID, now-300, // 5分以内の重複更新をスキップ
	)
	return err
}
