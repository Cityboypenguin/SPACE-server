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

func txFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx, ok
}

// dbtx is the common subset of *sql.DB and *sql.Tx used by repositories.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// extractDB returns the tx from ctx if one is present; otherwise returns db.
func extractDB(ctx context.Context, db *sql.DB) dbtx {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return db
}

// inTx は ctx に乗っているトランザクションに参加し、無ければ自分で開いて fn を走らせる。
//
// 1つの書き込みが複数の表にまたがるのに、呼び出し側がトランザクションを張るとは
// 限らない口（新規登録の users + user_accounts など）で使う。途中で失敗したときに
// 片方の表だけ書かれた状態を残さないため。
func inTx(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(withTx(ctx, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
