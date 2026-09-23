package ent

import (
	"context"
	"database/sql"

	"entgo.io/ent/dialect"
)

// TxDriver exposes the transaction driver operations needed by the small set
// of services that execute dialect-aware SQL inside an Ent transaction.
type TxDriver interface {
	dialect.Driver
	Dialect() string
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (tx *Tx) Driver() TxDriver {
	return tx.config.driver.(TxDriver)
}
