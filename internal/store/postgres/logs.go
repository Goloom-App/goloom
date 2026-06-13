package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) InsertLogEntry(ctx context.Context, e domain.LogEntry) error {
	attrsJSON, err := json.Marshal(e.Attributes)
	if err != nil {
		attrsJSON = []byte("{}")
	}
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	_, err = s.pool.Exec(ctx, `
		insert into log_entries (id, level, message, attributes_json, source_file, source_line, created_at, archived_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, e.Level, e.Message, string(attrsJSON), e.SourceFile, e.SourceLine, e.CreatedAt, e.ArchivedAt)
	return err
}

func (s *Store) ListLogEntries(ctx context.Context, filter domain.LogFilter) ([]domain.LogEntry, error) {
	where, args := buildLogWhere(filter)
	query := fmt.Sprintf(`select id, level, message, attributes_json, source_file, source_line, created_at, archived_at
		from log_entries %s order by created_at desc limit $%d offset $%d`,
		where, len(args)+1, len(args)+2)
	args = append(args, filterLimit(filter.Limit), filter.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogEntries(rows)
}

func (s *Store) CountLogEntries(ctx context.Context, filter domain.LogFilter) (int, error) {
	where, args := buildLogWhere(filter)
	var total int
	err := s.pool.QueryRow(ctx, `select count(*) from log_entries `+where, args...).Scan(&total)
	return total, err
}

func (s *Store) ArchiveLogEntry(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `update log_entries set archived_at = now() where id = $1`, id)
	return err
}

func (s *Store) UnarchiveLogEntry(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `update log_entries set archived_at = null where id = $1`, id)
	return err
}

func (s *Store) DeleteLogEntry(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `delete from log_entries where id = $1`, id)
	return err
}

func (s *Store) DeleteLogEntriesBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.pool.Exec(ctx, `delete from log_entries where created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

// --- helpers ---

func buildLogWhere(f domain.LogFilter) (string, []any) {
	var clauses []string
	var args []any

	if f.Level != "" {
		clauses = append(clauses, fmt.Sprintf("level = $%d", len(args)+1))
		args = append(args, f.Level)
	}
	if f.Search != "" {
		clauses = append(clauses, fmt.Sprintf("(message ilike $%d or attributes_json::text ilike $%d)", len(args)+1, len(args)+2))
		args = append(args, "%"+f.Search+"%")
	}
	if f.Archived != nil {
		if *f.Archived {
			clauses = append(clauses, "archived_at is not null")
		} else {
			clauses = append(clauses, "archived_at is null")
		}
	}
	if f.Before != nil {
		clauses = append(clauses, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *f.Before)
	}
	if f.After != nil {
		clauses = append(clauses, fmt.Sprintf("created_at > $%d", len(args)+1))
		args = append(args, *f.After)
	}

	where := ""
	if len(clauses) > 0 {
		where = " where "
		for i, c := range clauses {
			if i > 0 {
				where += " and "
			}
			where += c
		}
	}
	return where, args
}

func filterLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}

func scanLogEntries(rows pgx.Rows) ([]domain.LogEntry, error) {
	entries := make([]domain.LogEntry, 0)
	for rows.Next() {
		var e domain.LogEntry
		var attrsJSON []byte
		if err := rows.Scan(&e.ID, &e.Level, &e.Message, &attrsJSON, &e.SourceFile, &e.SourceLine,
			&e.CreatedAt, &e.ArchivedAt); err != nil {
			return nil, err
		}
		if len(attrsJSON) > 0 && string(attrsJSON) != "{}" {
			_ = json.Unmarshal(attrsJSON, &e.Attributes)
		}
		if e.Attributes == nil {
			e.Attributes = make(map[string]string)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
