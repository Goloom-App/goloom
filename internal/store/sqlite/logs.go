package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) InsertLogEntry(ctx context.Context, e domain.LogEntry) error {
	attrsJSON, err := json.Marshal(e.Attributes)
	if err != nil {
		attrsJSON = []byte("{}")
	}
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	_, err = s.db.ExecContext(ctx, `
		insert into log_entries (id, level, message, attributes_json, source_file, source_line, created_at, archived_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Level, e.Message, string(attrsJSON), e.SourceFile, e.SourceLine,
		formatTime(e.CreatedAt), formatTimeOptional(e.ArchivedAt))
	return err
}

func (s *Store) ListLogEntries(ctx context.Context, filter domain.LogFilter) ([]domain.LogEntry, error) {
	where, args := buildLogWhere(filter)
	query := `select id, level, message, attributes_json, source_file, source_line, created_at, archived_at
		from log_entries ` + where + ` order by created_at desc limit ? offset ?`
	args = append(args, filterLimit(filter.Limit), filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogEntries(rows)
}

func (s *Store) CountLogEntries(ctx context.Context, filter domain.LogFilter) (int, error) {
	where, args := buildLogWhere(filter)
	var total int
	err := s.db.QueryRowContext(ctx, `select count(*) from log_entries `+where, args...).Scan(&total)
	return total, err
}

func (s *Store) ArchiveLogEntry(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `update log_entries set archived_at = ? where id = ?`,
		formatTime(time.Now().UTC()), id)
	return err
}

func (s *Store) UnarchiveLogEntry(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `update log_entries set archived_at = null where id = ?`, id)
	return err
}

func (s *Store) DeleteLogEntry(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `delete from log_entries where id = ?`, id)
	return err
}

func (s *Store) DeleteLogEntriesBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `delete from log_entries where created_at < ?`, formatTime(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- helpers ---

func buildLogWhere(f domain.LogFilter) (string, []any) {
	var clauses []string
	var args []any

	if f.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, f.Level)
	}
	if f.Search != "" {
		clauses = append(clauses, "(message like ? or attributes_json like ?)")
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
		clauses = append(clauses, "created_at < ?")
		args = append(args, formatTime(*f.Before))
	}
	if f.After != nil {
		clauses = append(clauses, "created_at > ?")
		args = append(args, formatTime(*f.After))
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

func scanLogEntries(rows *sql.Rows) ([]domain.LogEntry, error) {
	entries := make([]domain.LogEntry, 0)
	for rows.Next() {
		var e domain.LogEntry
		var attrsJSON string
		var archivedAt sql.NullString
		if err := rows.Scan(&e.ID, &e.Level, &e.Message, &attrsJSON, &e.SourceFile, &e.SourceLine,
			&e.CreatedAt, &archivedAt); err != nil {
			return nil, err
		}
		if attrsJSON != "" && attrsJSON != "{}" {
			_ = json.Unmarshal([]byte(attrsJSON), &e.Attributes)
		}
		if e.Attributes == nil {
			e.Attributes = make(map[string]string)
		}
		if archivedAt.Valid {
			t, err := time.Parse(sqliteTimeLayout, archivedAt.String)
			if err == nil {
				e.ArchivedAt = &t
			}
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func formatTimeOptional(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
