package postgres

import (
	"context"
	"errors"
	"fmt"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

const pushSubscriptionColumns = `s.id::text, s.user_id::text, s.endpoint, s.p256dh, s.auth, s.enabled, s.created_at, s.updated_at`

func (s *Store) CountOpenReviewItems(ctx context.Context, teamID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		select count(*)
		from scheduled_posts
		where team_id = $1
		  and status = $2
		  and source = $3`,
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
	err := s.pool.QueryRow(ctx, `
		select count(*)
		from scheduled_posts p
		join team_memberships m
		  on m.team_id = p.team_id
		 and m.user_id = $1
		 and m.role in ('owner', 'editor')
		where p.status = $2
		  and p.source = $3`,
		userID, domain.PostStatusDraft, domain.PostSourceAutomation,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountUserOpenReviewItems: %w", err)
	}
	return count, nil
}

func (s *Store) CreatePushSubscription(ctx context.Context, userID string, sub domain.PushSubscription) (domain.PushSubscription, error) {
	const query = `
		insert into push_subscriptions as s (user_id, endpoint, p256dh, auth, enabled)
		values ($1, $2, $3, $4, true)
		on conflict(endpoint) do update set
			user_id = excluded.user_id,
			p256dh = excluded.p256dh,
			auth = excluded.auth,
			enabled = true,
			updated_at = now()
		returning ` + pushSubscriptionColumns
	row, err := scanPushSubscription(s.pool.QueryRow(ctx, query, userID, sub.Endpoint, sub.P256dh, sub.Auth))
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("CreatePushSubscription: %w", err)
	}
	return row, nil
}

func (s *Store) ListPushSubscriptions(ctx context.Context, userID string) ([]domain.PushSubscription, error) {
	const query = `
		select ` + pushSubscriptionColumns + `
		from push_subscriptions s
		where s.user_id = $1
		order by s.created_at asc`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("ListPushSubscriptions: %w", err)
	}
	defer rows.Close()
	return collectPushSubscriptions(rows)
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
	const query = `
		update push_subscriptions
		set enabled = $1, updated_at = now()
		where id = $2 and user_id = $3`
	tag, err := s.pool.Exec(ctx, query, enabled, subID, userID)
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("UpdatePushSubscriptionEnabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.PushSubscription{}, domain.ErrPushSubscriptionNotFound
	}
	return s.pushSubscriptionByID(ctx, subID)
}

func (s *Store) DeletePushSubscription(ctx context.Context, userID, subID string) error {
	tag, err := s.pool.Exec(ctx, `
		delete from push_subscriptions
		where id = $1 and user_id = $2`, subID, userID)
	if err != nil {
		return fmt.Errorf("DeletePushSubscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrPushSubscriptionNotFound
	}
	return nil
}

func (s *Store) RetirePushSubscription(ctx context.Context, subID string) error {
	_, err := s.pool.Exec(ctx, `
		delete from push_subscriptions
		where id = $1`, subID)
	if err != nil {
		return fmt.Errorf("RetirePushSubscription: %w", err)
	}
	return nil
}

func (s *Store) ListPushTargets(ctx context.Context, teamID string) ([]domain.PushSubscription, error) {
	const query = `
		select ` + pushSubscriptionColumns + `
		from push_subscriptions s
		join team_memberships m
		  on m.user_id = s.user_id
		 and m.team_id = $1
		 and m.role in ('owner', 'editor')
		left join team_notification_prefs p
		  on p.user_id = s.user_id
		 and p.team_id = $1
		where s.enabled = true
		  and coalesce(p.enabled, true) = true`
	rows, err := s.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("ListPushTargets: %w", err)
	}
	defer rows.Close()
	return collectPushSubscriptions(rows)
}

func (s *Store) SetTeamNotificationPref(ctx context.Context, userID, teamID string, enabled bool) (domain.TeamNotificationPref, error) {
	const query = `
		insert into team_notification_prefs (user_id, team_id, enabled)
		values ($1, $2, $3)
		on conflict(user_id, team_id) do update set
			enabled = excluded.enabled,
			updated_at = now()
		returning user_id::text, team_id::text, enabled, created_at, updated_at`
	var pref domain.TeamNotificationPref
	if err := s.pool.QueryRow(ctx, query, userID, teamID, enabled).Scan(
		&pref.UserID, &pref.TeamID, &pref.Enabled, &pref.CreatedAt, &pref.UpdatedAt,
	); err != nil {
		return domain.TeamNotificationPref{}, fmt.Errorf("SetTeamNotificationPref: %w", err)
	}
	return pref, nil
}

func (s *Store) ListTeamNotificationPrefs(ctx context.Context, userID string) ([]domain.TeamNotificationPref, error) {
	rows, err := s.pool.Query(ctx, `
		select user_id::text, team_id::text, enabled, created_at, updated_at
		from team_notification_prefs
		where user_id = $1
		order by created_at asc`, userID)
	if err != nil {
		return nil, fmt.Errorf("ListTeamNotificationPrefs: %w", err)
	}
	defer rows.Close()
	var out []domain.TeamNotificationPref
	for rows.Next() {
		var pref domain.TeamNotificationPref
		if err := rows.Scan(&pref.UserID, &pref.TeamID, &pref.Enabled, &pref.CreatedAt, &pref.UpdatedAt); err != nil {
			return nil, fmt.Errorf("ListTeamNotificationPrefs: scan: %w", err)
		}
		out = append(out, pref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetVAPIDKeys(ctx context.Context) (domain.VAPIDKeys, error) {
	var keys domain.VAPIDKeys
	err := s.pool.QueryRow(ctx, `
		select public_key, private_key, updated_at
		from vapid_keys
		where id = 1`).Scan(&keys.PublicKey, &keys.PrivateKey, &keys.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.VAPIDKeys{}, domain.ErrVAPIDKeysNotSet
		}
		return domain.VAPIDKeys{}, fmt.Errorf("GetVAPIDKeys: %w", err)
	}
	return keys, nil
}

func (s *Store) UpsertVAPIDKeys(ctx context.Context, keys domain.VAPIDKeys) error {
	_, err := s.pool.Exec(ctx, `
		insert into vapid_keys (id, public_key, private_key, updated_at)
		values (1, $1, $2, $3)
		on conflict(id) do update set
			public_key = excluded.public_key,
			private_key = excluded.private_key,
			updated_at = excluded.updated_at`,
		keys.PublicKey, keys.PrivateKey, keys.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("UpsertVAPIDKeys: %w", err)
	}
	return nil
}

func (s *Store) pushSubscriptionByID(ctx context.Context, subID string) (domain.PushSubscription, error) {
	const query = `
		select ` + pushSubscriptionColumns + `
		from push_subscriptions s
		where s.id = $1`
	sub, err := scanPushSubscription(s.pool.QueryRow(ctx, query, subID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PushSubscription{}, domain.ErrPushSubscriptionNotFound
		}
		return domain.PushSubscription{}, fmt.Errorf("pushSubscriptionByID: %w", err)
	}
	return sub, nil
}

func scanPushSubscription(row interface{ Scan(dest ...any) error }) (domain.PushSubscription, error) {
	var sub domain.PushSubscription
	if err := row.Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Endpoint,
		&sub.P256dh,
		&sub.Auth,
		&sub.Enabled,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	); err != nil {
		return domain.PushSubscription{}, err
	}
	return sub, nil
}

func collectPushSubscriptions(rows pgx.Rows) ([]domain.PushSubscription, error) {
	var out []domain.PushSubscription
	for rows.Next() {
		sub, err := scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}