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
	_, err = s.pool.Exec(ctx, `
		insert into audit_events (id, team_id, actor_user_id, actor_name, actor_email, actor_kind,
			token_id, token_name, action, target_type, target_id, summary, metadata_json, created_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		e.ID, e.TeamID, e.ActorUserID, e.ActorName, e.ActorEmail, e.ActorKind,
		e.TokenID, e.TokenName, e.Action, e.TargetType, e.TargetID,
		e.Summary, string(metaJSON), e.CreatedAt)
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, error) {
	where, args := buildAuditWhere(filter)
	query := fmt.Sprintf(`select id, team_id, actor_user_id, actor_name, actor_email, actor_kind,
		token_id, token_name, action, target_type, target_id, summary, metadata_json, created_at
		from audit_events %s order by created_at desc limit $%d offset $%d`,
		where, len(args)+1, len(args)+2)
	args = append(args, filterLimit(filter.Limit), filter.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

func (s *Store) CountAuditEvents(ctx context.Context, filter domain.AuditFilter) (int, error) {
	where, args := buildAuditWhere(filter)
	var total int
	err := s.pool.QueryRow(ctx, `select count(*) from audit_events `+where, args...).Scan(&total)
	return total, err
}

// --- helpers ---

func buildAuditWhere(f domain.AuditFilter) (string, []any) {
	clauses := []string{"team_id = $1"}
	args := []any{f.TeamID}

	if f.ActorUserID != "" {
		clauses = append(clauses, fmt.Sprintf("actor_user_id = $%d", len(args)+1))
		args = append(args, f.ActorUserID)
	}
	if f.Action != "" {
		clauses = append(clauses, fmt.Sprintf("action = $%d", len(args)+1))
		args = append(args, f.Action)
	}
	if f.Before != nil {
		clauses = append(clauses, fmt.Sprintf("created_at < $%d", len(args)+1))
		args = append(args, *f.Before)
	}
	if f.After != nil {
		clauses = append(clauses, fmt.Sprintf("created_at > $%d", len(args)+1))
		args = append(args, *f.After)
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

func scanAuditEvents(rows pgx.Rows) ([]domain.AuditEvent, error) {
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var e domain.AuditEvent
		var tokenID, tokenName, targetID *string
		var metaJSON []byte
		if err := rows.Scan(&e.ID, &e.TeamID, &e.ActorUserID, &e.ActorName, &e.ActorEmail, &e.ActorKind,
			&tokenID, &tokenName, &e.Action, &e.TargetType, &targetID, &e.Summary, &metaJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.TokenID = tokenID
		e.TokenName = tokenName
		e.TargetID = targetID
		if len(metaJSON) > 0 && string(metaJSON) != "{}" {
			_ = json.Unmarshal(metaJSON, &e.Metadata)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
