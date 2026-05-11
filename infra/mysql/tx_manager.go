package mysql

import (
	"context"
	"database/sql"
)

type MySQLTxManager struct {
	DB *sql.DB
}

func NewMySQLTxManager(db *sql.DB) *MySQLTxManager {
	return &MySQLTxManager{DB: db}
}

func (m *MySQLTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(withTx(ctx, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
