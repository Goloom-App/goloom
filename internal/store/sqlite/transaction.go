package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store/txcontext"
	"strings"

	"github.com/google/uuid"
)

type sqliteTxKey struct{ db *contextDB }
type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type sqlTransaction interface {
	sqlExecutor
	Commit() error
	Rollback() error
}

// contextDB retains the original Store receiver (and decorators) while routing
// each query to the transaction carried by its context. Never share a tx globally.
type contextDB struct{ *sql.DB }

func (db *contextDB) executor(ctx context.Context) sqlExecutor {
	if tx, ok := ctx.Value(sqliteTxKey{db}).(*sql.Tx); ok {
		return tx
	}
	return db.DB
}
func (db *contextDB) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return db.executor(ctx).ExecContext(ctx, q, args...)
}
func (db *contextDB) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return db.executor(ctx).QueryContext(ctx, q, args...)
}
func (db *contextDB) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return db.executor(ctx).QueryRowContext(ctx, q, args...)
}
func (db *contextDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (sqlTransaction, error) {
	if tx, ok := ctx.Value(sqliteTxKey{db}).(*sql.Tx); ok {
		name := "audit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := tx.ExecContext(ctx, "savepoint "+name); err != nil {
			return nil, err
		}
		return &sqliteSavepoint{tx: tx, ctx: ctx, name: name}, nil
	}
	return db.DB.BeginTx(ctx, opts)
}

type sqliteSavepoint struct {
	tx   *sql.Tx
	ctx  context.Context
	name string
	done bool
}

func (s *sqliteSavepoint) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.tx.ExecContext(ctx, q, args...)
}
func (s *sqliteSavepoint) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.tx.QueryContext(ctx, q, args...)
}
func (s *sqliteSavepoint) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return s.tx.QueryRowContext(ctx, q, args...)
}
func (s *sqliteSavepoint) Commit() error {
	if s.done {
		return sql.ErrTxDone
	}
	s.done = true
	_, err := s.tx.ExecContext(s.ctx, "release savepoint "+s.name)
	return err
}
func (s *sqliteSavepoint) Rollback() error {
	if s.done {
		return sql.ErrTxDone
	}
	s.done = true
	ctx := context.WithoutCancel(s.ctx)
	_, err := s.tx.ExecContext(ctx, "rollback to savepoint "+s.name)
	if err != nil {
		return err
	}
	_, err = s.tx.ExecContext(ctx, "release savepoint "+s.name)
	return err
}
func (s *Store) WithAuditTransaction(ctx context.Context, fn func(context.Context) error) error {
	hookctx, finish := txcontext.Begin(ctx)
	committed := false
	defer func() { finish(committed) }()
	ctx = hookctx

	if _, ok := ctx.Value(sqliteTxKey{s.db}).(*sql.Tx); ok {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := fn(ctx); err != nil {
			return err
		}
		err = tx.Commit()
		committed = err == nil
		return err
	}
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	txctx := context.WithValue(ctx, sqliteTxKey{s.db}, tx)
	if err := fn(txctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err = tx.Commit()
	committed = err == nil
	if errors.Is(err, sql.ErrTxDone) && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// LockAutomationSource serializes work by source row without retaining a lease
// after commit, so deliberate regeneration remains possible.
func (s *Store) LockAutomationSource(ctx context.Context, kind, id string) error {
	if _, ok := ctx.Value(sqliteTxKey{s.db}).(*sql.Tx); !ok {
		return fmt.Errorf("automation source lock requires active transaction")
	}
	var table string
	switch kind {
	case "rss_feed":
		table = "rss_feed_configs"
	case "post_template":
		table = "post_templates"
	default:
		return fmt.Errorf("invalid automation source kind")
	}
	result, err := s.db.ExecContext(ctx, "update "+table+" set id=id where id=?", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("automation source: %w", domain.ErrNotFound)
	}
	return nil
}
