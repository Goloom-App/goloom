package sqlite

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
	_, err = s.db.ExecContext(ctx, `insert into audit_pending(id, team_id, payload) values (?, ?, ?) on conflict(id) do nothing`, e.ID, e.TeamID, string(payload))
	return err
}
func (s *Store) DrainAuditEvents(ctx context.Context) error {
	return s.WithAuditTransaction(ctx, func(txctx context.Context) error {
		rows, err := s.db.QueryContext(txctx, `select id, payload from audit_pending order by id limit 100`)
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
			if _, err := s.db.ExecContext(txctx, `delete from audit_pending where id = ?`, e.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) ClaimPostTargetPublication(ctx context.Context, postID, accountID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `update scheduled_post_targets set status = 'processing', last_error = 'publication_outcome_unknown' where post_id = ? and account_id = ? and status in ('pending','failed')`, postID, accountID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// ClaimAIJobCompletion permits one terminal transition; retries cannot repeat side effects.
func (s *Store) ClaimAIJobCompletion(ctx context.Context, jobID string, status domain.AIJobStatus, result []byte, errorMessage string) (bool, error) {
	if status != domain.AIJobStatusCompleted && status != domain.AIJobStatusFailed {
		return false, fmt.Errorf("AI completion requires terminal status")
	}

	var resultValue any
	if len(result) > 0 {
		resultValue = string(result)
	}
	now := nowString()
	r, err := s.db.ExecContext(ctx, `update ai_jobs set status=?,result=?,error_message=?,updated_at=?,completed_at=? where id=? and status in ('pending','processing')`, status, resultValue, nullableString(errorMessage), now, now, jobID)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}
