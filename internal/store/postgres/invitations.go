package postgres

import (
	"context"
	"errors"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) ListTeamInvitations(ctx context.Context, teamID string) ([]domain.TeamInvitation, error) {
	rows, err := s.pool.Query(ctx, `
		select id, team_id, email, role, expires_at, created_by_user_id, created_at
		from team_invitations
		where team_id = $1
		order by created_at desc, id`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invitations []domain.TeamInvitation
	for rows.Next() {
		var inv domain.TeamInvitation
		if err := rows.Scan(&inv.ID, &inv.TeamID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedByUserID, &inv.CreatedAt); err != nil {
			return nil, err
		}
		invitations = append(invitations, inv)
	}
	return invitations, rows.Err()
}

func (s *Store) DeleteTeamInvitation(ctx context.Context, teamID, invitationID string) error {
	tag, err := s.pool.Exec(ctx, `delete from team_invitations where id = $1 and team_id = $2`, invitationID, teamID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("invitation not found")
	}
	return nil
}
