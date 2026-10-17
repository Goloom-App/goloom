package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
	"time"
)

func (s *Store) EnqueueAuditEvent(ctx context.Context, e domain.AuditEvent) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `insert into audit_pending(id, team_id, payload) values ($1, $2, $3) on conflict(id) do nothing`, e.ID, e.TeamID, string(payload))
	return err
}
func (s *Store) DrainAuditEvents(ctx context.Context) error {
	return s.WithAuditTransaction(ctx, func(txctx context.Context) error {
		rows, err := s.pool.Query(txctx, `select id, payload from audit_pending order by id limit 100 for update skip locked`)
		if err != nil {
			return err
		}
		// Close the cursor before inserting: SQLite and pgx share this connection.
		events := make([]domain.AuditEvent, 0)
		for rows.Next() {
			var id, payload string
			if err := rows.Scan(&id, &payload); err != nil {
				rows.Close()
				return err
			}
			var e domain.AuditEvent
			if err := json.Unmarshal([]byte(payload), &e); err != nil {
				rows.Close()
				return fmt.Errorf("decode pending audit event: %w", err)
			}
			if e.ID != id {
				rows.Close()
				return fmt.Errorf("pending audit event ID mismatch")
			}
			events = append(events, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, e := range events {
			if err := s.InsertAuditEvent(txctx, e); err != nil {
				return fmt.Errorf("deliver pending audit event: %w", err)
			}
			if _, err := s.pool.Exec(txctx, `delete from audit_pending where id = $1`, e.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) ClaimPostTargetPublication(ctx context.Context, postID, accountID string) (bool, error) {
	result, err := s.pool.Exec(ctx, `update scheduled_post_targets set status = 'processing', last_error = 'publication_outcome_unknown' where post_id = $1 and account_id = $2 and status in ('pending','failed')`, postID, accountID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}
