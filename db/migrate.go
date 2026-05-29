package db

import (
	"database/sql"
	"embed"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func RunMigrations(db *sql.DB) error {
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return err
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
