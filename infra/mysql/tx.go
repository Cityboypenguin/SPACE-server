package mysql

import (
	"context"
	"database/sql"
)

type txContextKey struct{}

// withTx stores tx in ctx so repositories can pick it up.
func withTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

// dbtx is the common subset of *sql.DB and *sql.Tx used by repositories.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// extractDB returns the tx from ctx if one is present; otherwise returns db.
func extractDB(ctx context.Context, db *sql.DB) dbtx {
	if tx, ok := ctx.Value(txContextKey{}).(*sql.Tx); ok {
		return tx
	}
	return db
}
