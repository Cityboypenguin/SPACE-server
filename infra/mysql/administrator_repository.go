package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MySQLAdministratorRepository struct {
	db *sql.DB
}

func NewMySQLAdministratorRepository(db *sql.DB) repository.AdministratorRepository {
	return &MySQLAdministratorRepository{
		db: db,
	}
}

// Implement the methods of the AdministratorRepository interface here
func (r *MySQLAdministratorRepository) SaveAdministrator(ctx context.Context, a *model.Administrator) error {
	now := time.Now()

	if a.ID == 0 {
		a.CreatedAt = now
		a.UpdatedAt = now
		id, err := r.CreateAdministrator(ctx, a)
		if err != nil {
			return err
		}

		a.ID = id
	} else {
		if err := r.UpdateAdministrator(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

func (r *MySQLAdministratorRepository) CreateAdministrator(ctx context.Context, a *model.Administrator) (int64, error) {
	query := `
		INSERT INTO administrators (name, email, hashed_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := r.db.ExecContext(ctx, query, a.Name, a.Email, a.HashedPassword, a.CreatedAt.Unix(), a.UpdatedAt.Unix())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *MySQLAdministratorRepository) GetAdministratorByID(ctx context.Context, id int64) (*model.Administrator, error) {
	query := `
		SELECT id, name, email, hashed_password, created_at, updated_at
		FROM administrators
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var a model.Administrator
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(
		&a.ID,
		&a.Name,
		&a.Email,
		&a.HashedPassword,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		return nil, err
	}
	a.CreatedAt = time.Unix(createdAtUnix, 0)
	a.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &a, nil
}

func (r *MySQLAdministratorRepository) DeleteAdministrator(ctx context.Context, id int64) (bool, error) {
	query := `
		DELETE FROM administrators
		WHERE id = ?
	`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected > 0, nil
}

func (r *MySQLAdministratorRepository) ListAdministrators(ctx context.Context) ([]*model.Administrator, error) {
	query := `
		SELECT id, name, email, hashed_password, created_at, updated_at
		FROM administrators
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var administrators []*model.Administrator
	for rows.Next() {
		var a model.Administrator
		var createdAtUnix, updatedAtUnix int64
		if err := rows.Scan(
			&a.ID,
			&a.Name,
			&a.Email,
			&a.HashedPassword,
			&createdAtUnix,
			&updatedAtUnix,
		); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(createdAtUnix, 0)
		a.UpdatedAt = time.Unix(updatedAtUnix, 0)
		administrators = append(administrators, &a)
	}
	return administrators, nil
}

func (r *MySQLAdministratorRepository) FindByEmail(ctx context.Context, email string) (*model.Administrator, error) {
	query := `
		SELECT id, name, email, hashed_password, created_at, updated_at
		FROM administrators
		WHERE email = ?
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, email)

	var a model.Administrator
	var createdAtUnix, updatedAtUnix int64
	if err := row.Scan(
		&a.ID,
		&a.Name,
		&a.Email,
		&a.HashedPassword,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.CreatedAt = time.Unix(createdAtUnix, 0)
	a.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &a, nil
}

func (r *MySQLAdministratorRepository) UpdateAdministrator(ctx context.Context, a *model.Administrator) error {
	a.UpdatedAt = time.Now()

	query := `
		UPDATE administrators
		SET name = ?, email = ?, hashed_password = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.db.ExecContext(
		ctx,
		query,
		a.Name,
		a.Email,
		a.HashedPassword,
		a.UpdatedAt.Unix(),
		a.ID,
	)
	return err
}

func (r *MySQLAdministratorRepository) SearchAdministratorsByName(ctx context.Context, name string) ([]*model.Administrator, error) {
	query := `
		SELECT id, name, email, hashed_password, created_at, updated_at
		FROM administrators
		WHERE name LIKE ?
	`
	rows, err := r.db.QueryContext(ctx, query, "%"+name+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var administrators []*model.Administrator
	for rows.Next() {
		var a model.Administrator
		var createdAtUnix, updatedAtUnix int64
		if err := rows.Scan(
			&a.ID,
			&a.Name,
			&a.Email,
			&a.HashedPassword,
			&createdAtUnix,
			&updatedAtUnix,
		); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(createdAtUnix, 0)
		a.UpdatedAt = time.Unix(updatedAtUnix, 0)
		administrators = append(administrators, &a)
	}
	return administrators, nil
}
