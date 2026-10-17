package postgres

import (
	"context"
	"git.f4mily.net/goloom/internal/store/txcontext"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresTxKey struct{ pool *contextPool }
type pgExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// contextPool preserves wrappers around Store: callback calls continue through
// their original receiver while SQL joins the context's transaction.
type contextPool struct{ *pgxpool.Pool }

func (p *contextPool) executor(ctx context.Context) pgExecutor {
	if tx, ok := ctx.Value(postgresTxKey{p}).(pgx.Tx); ok {
		return tx
	}
	return p.Pool
}
func (p *contextPool) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	return p.executor(ctx).Exec(ctx, q, args...)
}
func (p *contextPool) Query(ctx context.Context, q string, args ...any) (pgx.Rows, error) {
	return p.executor(ctx).Query(ctx, q, args...)
}
func (p *contextPool) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	return p.executor(ctx).QueryRow(ctx, q, args...)
}
func (p *contextPool) Begin(ctx context.Context) (pgx.Tx, error) {
	if tx, ok := ctx.Value(postgresTxKey{p}).(pgx.Tx); ok {
		return tx.Begin(ctx)
	}
	return p.Pool.Begin(ctx)
}
func (s *Store) WithAuditTransaction(ctx context.Context, fn func(context.Context) error) error {
	hookctx, finish := txcontext.Begin(ctx)
	committed := false
	defer func() { finish(committed) }()
	ctx = hookctx

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	txctx := context.WithValue(ctx, postgresTxKey{s.pool}, tx)
	if err := fn(txctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err = tx.Commit(ctx)
	committed = err == nil
	return err
}
