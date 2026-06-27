package postgres

import (
	"context"

	"git.f4mily.net/goloom/internal/domain"
)

// AdminListPublishFailures returns failed, not-yet-acknowledged posts with their
// per-account results, newest first.
func (s *Store) AdminListPublishFailures(ctx context.Context) ([]domain.PublishFailure, error) {
	rows, err := s.pool.Query(ctx, `
		select p.id, p.team_id, coalesce(t.name, ''), p.title, p.scheduled_at,
		       p.attempt_count, coalesce(p.last_error, ''), p.updated_at
		from scheduled_posts p
		left join teams t on t.id = p.team_id
		where p.status = 'failed' and p.acknowledged_at is null
		order by p.updated_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var failures []domain.PublishFailure
	index := map[string]int{}
	for rows.Next() {
		var f domain.PublishFailure
		if err := rows.Scan(&f.PostID, &f.TeamID, &f.TeamName, &f.Title, &f.ScheduledAt,
			&f.AttemptCount, &f.LastError, &f.UpdatedAt); err != nil {
			return nil, err
		}
		index[f.PostID] = len(failures)
		failures = append(failures, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(failures) == 0 {
		return failures, nil
	}

	trows, err := s.pool.Query(ctx, `
		select tg.post_id, tg.account_id, coalesce(a.name, ''), coalesce(a.username, ''),
		       coalesce(a.provider, ''), tg.status, coalesce(tg.last_error, ''), coalesce(tg.published_url, '')
		from scheduled_post_targets tg
		join scheduled_posts p on p.id = tg.post_id
		left join social_accounts a on a.id = tg.account_id
		where p.status = 'failed' and p.acknowledged_at is null`)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var postID, name, username string
		var tgt domain.PublishFailureTarget
		if err := trows.Scan(&postID, &tgt.AccountID, &name, &username, &tgt.Provider,
			&tgt.Status, &tgt.LastError, &tgt.PublishedURL); err != nil {
			return nil, err
		}
		tgt.AccountName = name
		if tgt.AccountName == "" {
			tgt.AccountName = username
		}
		if i, ok := index[postID]; ok {
			failures[i].Targets = append(failures[i].Targets, tgt)
		}
	}
	return failures, trows.Err()
}

// AdminAcknowledgeFailedPost marks a failed post as reviewed so it no longer
// counts as needing attention.
func (s *Store) AdminAcknowledgeFailedPost(ctx context.Context, postID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		update scheduled_posts set acknowledged_at = now(), updated_at = now()
		where id = $1 and status = 'failed' and acknowledged_at is null`, postID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// AdminRetryFailedPost re-queues a failed post for publication. Targets that
// already published successfully are left untouched to avoid double-posting.
func (s *Store) AdminRetryFailedPost(ctx context.Context, postID string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		update scheduled_posts
		set status = 'pending', attempt_count = 0, last_error = null, acknowledged_at = null,
		    scheduled_at = now(), updated_at = now()
		where id = $1 and status = 'failed'`, postID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		update scheduled_post_targets set status = 'pending', last_error = null
		where post_id = $1 and status <> 'posted'`, postID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
