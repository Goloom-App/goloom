package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func (s *Store) InsertAuditEvent(ctx context.Context, e domain.AuditEvent) error {
	metaJSON, err := json.Marshal(e.Metadata)
	if err != nil || len(e.Metadata) == 0 {
		metaJSON = []byte("{}")
	}
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx, `
		insert into audit_events (id, team_id, actor_user_id, actor_name, actor_email, actor_kind,
			token_id, token_name, action, target_type, target_id, summary, metadata_json, created_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.TeamID, e.ActorUserID, e.ActorName, e.ActorEmail, e.ActorKind,
		nullablePtr(e.TokenID), nullablePtr(e.TokenName),
		e.Action, e.TargetType, nullablePtr(e.TargetID),
		e.Summary, string(metaJSON), formatTime(e.CreatedAt))
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, error) {
	where, args := buildAuditWhere(filter)
	query := `select id, team_id, actor_user_id, actor_name, actor_email, actor_kind,
		token_id, token_name, action, target_type, target_id, summary, metadata_json, created_at
		from audit_events ` + where + ` order by created_at desc limit ? offset ?`
	args = append(args, filterLimit(filter.Limit), filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

func (s *Store) CountAuditEvents(ctx context.Context, filter domain.AuditFilter) (int, error) {
	where, args := buildAuditWhere(filter)
	var total int
	err := s.db.QueryRowContext(ctx, `select count(*) from audit_events `+where, args...).Scan(&total)
	return total, err
}

// --- helpers ---

func buildAuditWhere(f domain.AuditFilter) (string, []any) {
	clauses := []string{"team_id = ?"}
	args := []any{f.TeamID}

	if f.ActorUserID != "" {
		clauses = append(clauses, "actor_user_id = ?")
		args = append(args, f.ActorUserID)
	}
	if f.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, f.Action)
	}
	if f.Before != nil {
		clauses = append(clauses, "created_at < ?")
		args = append(args, formatTime(*f.Before))
	}
	if f.After != nil {
		clauses = append(clauses, "created_at > ?")
		args = append(args, formatTime(*f.After))
	}

	where := " where "
	for i, c := range clauses {
		if i > 0 {
			where += " and "
		}
		where += c
	}
	return where, args
}

func scanAuditEvents(rows *sql.Rows) ([]domain.AuditEvent, error) {
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var e domain.AuditEvent
		var tokenID, tokenName, targetID sql.NullString
		var metaJSON string
		var createdAt string
		if err := rows.Scan(&e.ID, &e.TeamID, &e.ActorUserID, &e.ActorName, &e.ActorEmail, &e.ActorKind,
			&tokenID, &tokenName, &e.Action, &e.TargetType, &targetID, &e.Summary, &metaJSON, &createdAt); err != nil {
			return nil, err
		}
		if tokenID.Valid {
			v := tokenID.String
			e.TokenID = &v
		}
		if tokenName.Valid {
			v := tokenName.String
			e.TokenName = &v
		}
		if targetID.Valid {
			v := targetID.String
			e.TargetID = &v
		}
		if metaJSON != "" && metaJSON != "{}" {
			_ = json.Unmarshal([]byte(metaJSON), &e.Metadata)
		}
		e.CreatedAt, _ = parseTime(createdAt)
		events = append(events, e)
	}
	return events, rows.Err()
}

func nullablePtr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
