package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

const pushSubscriptionColumns = `id, user_id, endpoint, p256dh, auth, enabled, created_at, updated_at`

const pushSubscriptionColumnsQualified = `s.id, s.user_id, s.endpoint, s.p256dh, s.auth, s.enabled, s.created_at, s.updated_at`

func (s *Store) CountOpenReviewItems(ctx context.Context, teamID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		select count(*)
		from scheduled_posts
		where team_id = ?
		  and status = ?
		  and source = ?`,
		teamID, domain.PostStatusDraft, domain.PostSourceAutomation,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountOpenReviewItems: %w", err)
	}
	return count, nil
}

// CountUserOpenReviewItems sums the open review items across all teams where
// the user holds an owner/editor role — a viewer membership must not inflate
// the user's badge total. Mirrors the per-team review-counts semantics.
func (s *Store) CountUserOpenReviewItems(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		select count(*)
		from scheduled_posts p
		join team_memberships m
		  on m.team_id = p.team_id
		 and m.user_id = ?
		 and m.role in ('owner', 'editor')
		where p.status = ?
		  and p.source = ?`,
		userID, domain.PostStatusDraft, domain.PostSourceAutomation,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountUserOpenReviewItems: %w", err)
	}
	return count, nil
}

func (s *Store) CreatePushSubscription(ctx context.Context, userID string, sub domain.PushSubscription) (domain.PushSubscription, error) {
	id := uuid.NewString()
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into push_subscriptions (id, user_id, endpoint, p256dh, auth, enabled, created_at, updated_at)
		values (?, ?, ?, ?, ?, 1, ?, ?)
		on conflict(endpoint) do update set
			user_id = excluded.user_id,
			p256dh = excluded.p256dh,
			auth = excluded.auth,
			enabled = 1,
			updated_at = excluded.updated_at`,
		id, userID, sub.Endpoint, sub.P256dh, sub.Auth, now, now,
	)
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("CreatePushSubscription: %w", err)
	}
	return s.pushSubscriptionByEndpoint(ctx, sub.Endpoint)
}

func (s *Store) ListPushSubscriptions(ctx context.Context, userID string) ([]domain.PushSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		select `+pushSubscriptionColumns+`
		from push_subscriptions
		where user_id = ?
		order by created_at asc`, userID)
	if err != nil {
		return nil, fmt.Errorf("ListPushSubscriptions: %w", err)
	}
	defer rows.Close()
	var out []domain.PushSubscription
	for rows.Next() {
		sub, err := scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("ListPushSubscriptions: scan: %w", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetPushSubscription(ctx context.Context, userID, subID string) (domain.PushSubscription, error) {
	sub, err := s.pushSubscriptionByID(ctx, subID)
	if err != nil {
		return domain.PushSubscription{}, err
	}
	if sub.UserID != userID {
		return domain.PushSubscription{}, domain.ErrPushSubscriptionNotFound
	}
	return sub, nil
}

func (s *Store) UpdatePushSubscriptionEnabled(ctx context.Context, userID, subID string, enabled bool) (domain.PushSubscription, error) {
	now := nowString()
	res, err := s.db.ExecContext(ctx, `
		update push_subscriptions
		set enabled = ?, updated_at = ?
		where id = ? and user_id = ?`,
		boolToInt(enabled), now, subID, userID,
	)
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("UpdatePushSubscriptionEnabled: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.PushSubscription{}, err
	}
	if affected == 0 {
		return domain.PushSubscription{}, domain.ErrPushSubscriptionNotFound
	}
	return s.pushSubscriptionByID(ctx, subID)
}

func (s *Store) DeletePushSubscription(ctx context.Context, userID, subID string) error {
	res, err := s.db.ExecContext(ctx, `
		delete from push_subscriptions
		where id = ? and user_id = ?`, subID, userID)
	if err != nil {
		return fmt.Errorf("DeletePushSubscription: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrPushSubscriptionNotFound
	}
	return nil
}

func (s *Store) RetirePushSubscription(ctx context.Context, subID string) error {
	_, err := s.db.ExecContext(ctx, `
		delete from push_subscriptions
		where id = ?`, subID)
	if err != nil {
		return fmt.Errorf("RetirePushSubscription: %w", err)
	}
	return nil
}

func (s *Store) ListPushTargets(ctx context.Context, teamID string) ([]domain.PushSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		select `+pushSubscriptionColumnsQualified+`
		from push_subscriptions s
		join team_memberships m
		  on m.user_id = s.user_id
		 and m.team_id = ?
		 and m.role in ('owner', 'editor')
		left join team_notification_prefs p
		  on p.user_id = s.user_id
		 and p.team_id = ?
		where s.enabled = 1
		  and coalesce(p.enabled, 1) = 1`,
		teamID, teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListPushTargets: %w", err)
	}
	defer rows.Close()
	var out []domain.PushSubscription
	for rows.Next() {
		sub, err := scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("ListPushTargets: scan: %w", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SetTeamNotificationPref(ctx context.Context, userID, teamID string, enabled bool) (domain.TeamNotificationPref, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		insert into team_notification_prefs (user_id, team_id, enabled, created_at, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict(user_id, team_id) do update set
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		userID, teamID, boolToInt(enabled), now, now,
	)
	if err != nil {
		return domain.TeamNotificationPref{}, fmt.Errorf("SetTeamNotificationPref: %w", err)
	}
	var pref domain.TeamNotificationPref
	var enabledInt int
	var createdAt, updatedAt string
	err = s.db.QueryRowContext(ctx, `
		select user_id, team_id, enabled, created_at, updated_at
		from team_notification_prefs
		where user_id = ? and team_id = ?`, userID, teamID,
	).Scan(&pref.UserID, &pref.TeamID, &enabledInt, &createdAt, &updatedAt)
	if err != nil {
		return domain.TeamNotificationPref{}, fmt.Errorf("SetTeamNotificationPref: read back: %w", err)
	}
	pref.Enabled = enabledInt != 0
	pref.CreatedAt = mustParseTime(createdAt)
	pref.UpdatedAt = mustParseTime(updatedAt)
	return pref, nil
}

func (s *Store) ListTeamNotificationPrefs(ctx context.Context, userID string) ([]domain.TeamNotificationPref, error) {
	rows, err := s.db.QueryContext(ctx, `
		select user_id, team_id, enabled, created_at, updated_at
		from team_notification_prefs
		where user_id = ?
		order by created_at asc`, userID)
	if err != nil {
		return nil, fmt.Errorf("ListTeamNotificationPrefs: %w", err)
	}
	defer rows.Close()
	var out []domain.TeamNotificationPref
	for rows.Next() {
		var pref domain.TeamNotificationPref
		var enabledInt int
		var createdAt, updatedAt string
		if err := rows.Scan(&pref.UserID, &pref.TeamID, &enabledInt, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListTeamNotificationPrefs: scan: %w", err)
		}
		pref.Enabled = enabledInt != 0
		pref.CreatedAt = mustParseTime(createdAt)
		pref.UpdatedAt = mustParseTime(updatedAt)
		out = append(out, pref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetVAPIDKeys(ctx context.Context) (domain.VAPIDKeys, error) {
	var keys domain.VAPIDKeys
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		select public_key, private_key, updated_at
		from vapid_keys
		where id = 1`).Scan(&keys.PublicKey, &keys.PrivateKey, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.VAPIDKeys{}, domain.ErrVAPIDKeysNotSet
		}
		return domain.VAPIDKeys{}, fmt.Errorf("GetVAPIDKeys: %w", err)
	}
	keys.UpdatedAt = mustParseTime(updatedAt)
	return keys, nil
}

func (s *Store) UpsertVAPIDKeys(ctx context.Context, keys domain.VAPIDKeys) error {
	_, err := s.db.ExecContext(ctx, `
		insert into vapid_keys (id, public_key, private_key, updated_at)
		values (1, ?, ?, ?)
		on conflict(id) do update set
			public_key = excluded.public_key,
			private_key = excluded.private_key,
			updated_at = excluded.updated_at`,
		keys.PublicKey, keys.PrivateKey, formatTime(keys.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("UpsertVAPIDKeys: %w", err)
	}
	return nil
}

func (s *Store) pushSubscriptionByEndpoint(ctx context.Context, endpoint string) (domain.PushSubscription, error) {
	sub, err := scanPushSubscription(s.db.QueryRowContext(ctx, `
		select `+pushSubscriptionColumns+`
		from push_subscriptions
		where endpoint = ?`, endpoint))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.PushSubscription{}, domain.ErrPushSubscriptionNotFound
		}
		return domain.PushSubscription{}, fmt.Errorf("pushSubscriptionByEndpoint: %w", err)
	}
	return sub, nil
}

func (s *Store) pushSubscriptionByID(ctx context.Context, subID string) (domain.PushSubscription, error) {
	sub, err := scanPushSubscription(s.db.QueryRowContext(ctx, `
		select `+pushSubscriptionColumns+`
		from push_subscriptions
		where id = ?`, subID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.PushSubscription{}, domain.ErrPushSubscriptionNotFound
		}
		return domain.PushSubscription{}, fmt.Errorf("pushSubscriptionByID: %w", err)
	}
	return sub, nil
}

func scanPushSubscription(row interface{ Scan(dest ...any) error }) (domain.PushSubscription, error) {
	var (
		sub       domain.PushSubscription
		enabled   int
		createdAt string
		updatedAt string
	)
	if err := row.Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Endpoint,
		&sub.P256dh,
		&sub.Auth,
		&enabled,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domain.PushSubscription{}, err
	}
	sub.Enabled = enabled != 0
	sub.CreatedAt = mustParseTime(createdAt)
	sub.UpdatedAt = mustParseTime(updatedAt)
	return sub, nil
}