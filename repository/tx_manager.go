package repository

import "context"

// TxManager runs fn inside a single DB transaction.
// fn receives a derived context; repositories read the tx from that context via infra/mysql internals.
type TxManager interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
