package db

import (
	"database/sql"
	"embed"
	"errors"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// LatestVersion は埋め込んだ migration の最新の番号。同じコミットの app は起動時にここまで進む。
// 移行ツールは番号を書き込まず、これと DB の版が揃っていることを確かめる。
func LatestVersion() (uint, error) {
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return 0, err
	}
	defer source.Close()
	version, err := source.First()
	if err != nil {
		return 0, err
	}
	for {
		next, err := source.Next(version)
		if errors.Is(err, fs.ErrNotExist) {
			return version, nil
		}
		if err != nil {
			return 0, err
		}
		version = next
	}
}

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
