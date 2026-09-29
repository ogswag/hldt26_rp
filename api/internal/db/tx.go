package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// BeginTx starts a transaction on the query connection.
func (q *Queries) BeginTx(ctx context.Context) (pgx.Tx, error) {
	b, ok := q.db.(beginner)
	if !ok {
		return nil, fmt.Errorf("db.begin: transactions are not supported")
	}
	return b.Begin(ctx)
}
