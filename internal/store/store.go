package store

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/postgres"
	"git.f4mily.net/goloom/internal/store/sqlite"
)

type Store interface {
	Close()

	UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error)
	LookupAPIToken(ctx context.Context, bearerToken string) (domain.AuthenticatedPrincipal, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	SetUserAdmin(ctx context.Context, userID string, isAdmin bool) (domain.User, error)
	ListTeamsForUser(ctx context.Context, userID string, isAdmin bool) ([]domain.Team, error)
	EnsurePersonalTeam(ctx context.Context, userID string) (domain.Team, error)
	EnsurePersonalTeamsMigrated(ctx context.Context) error
	GetTeamByID(ctx context.Context, teamID string) (domain.Team, error)
	CreateTeam(ctx context.Context, ownerUserID string, input domain.CreateTeamInput) (domain.Team, error)
	ListTeamMembers(ctx context.Context, teamID string) ([]domain.TeamMembership, error)
	AddTeamMember(ctx context.Context, teamID string, input domain.AddTeamMemberInput) (domain.TeamMembership, error)
	RemoveTeamMember(ctx context.Context, teamID, userID string) error
	ListProviderInstances(ctx context.Context, providerName string) ([]domain.ProviderInstance, error)
	GetProviderInstanceByID(ctx context.Context, instanceID string) (domain.ProviderInstance, error)
	CreateProviderInstance(ctx context.Context, createdByUserID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error)
	UpdateProviderInstance(ctx context.Context, instanceID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error)
	UserHasAnyTeamRole(ctx context.Context, userID, teamID string, roles ...domain.TeamRole) (bool, error)
	ListTeamAccounts(ctx context.Context, teamID string) ([]domain.SocialAccount, error)
	CreateAccount(ctx context.Context, teamID string, input domain.ConnectedAccount) (domain.SocialAccount, error)
	DeleteAccount(ctx context.Context, teamID, accountID string) error
	DeleteSocialAccount(ctx context.Context, accountID string) error
	GetAccountByID(ctx context.Context, accountID string) (domain.SocialAccount, error)
	GetAccountsByIDsGlobal(ctx context.Context, ids []string) ([]domain.SocialAccount, error)
	GetAccountsByIDs(ctx context.Context, teamID string, ids []string) ([]domain.SocialAccount, error)
	MigrateAccountToTeam(ctx context.Context, userID string, accountID, targetTeamID string, isAdmin bool) error
	CreateTeamInvitation(ctx context.Context, teamID, createdByUserID string, input domain.CreateTeamInvitationInput) (domain.TeamInvitation, string, error)
	AcceptTeamInvitation(ctx context.Context, userID, email, rawToken string) (domain.TeamMembership, error)
	CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error)
	ListTeamPosts(ctx context.Context, teamID string) ([]domain.ScheduledPost, error)
	GetScheduledPost(ctx context.Context, teamID, postID string) (domain.ScheduledPost, error)
	GetScheduledPostByID(ctx context.Context, postID string) (domain.ScheduledPost, error)
	UpdateScheduledPost(ctx context.Context, teamID, postID string, input domain.CreatePostInput) (domain.ScheduledPost, error)
	CancelScheduledPost(ctx context.Context, teamID, postID string) error
	DeleteScheduledPost(ctx context.Context, teamID, postID string) error
	ListDuePosts(ctx context.Context, limit int) ([]domain.ScheduledPost, error)
	MarkPostProcessing(ctx context.Context, postID string) error
	MarkPostResult(ctx context.Context, postID string, attemptCount int, status domain.PostStatus, lastError string, nextAttempt *time.Time) error
	MarkPostTargetResult(ctx context.Context, postID, accountID string, status domain.PostStatus, publishedURL, lastError string) error
	LoadPostTargets(ctx context.Context, postID string) ([]domain.SocialAccount, error)
	DecryptAccessToken(account domain.SocialAccount) (string, error)
	DecryptRefreshToken(account domain.SocialAccount) (string, error)
	DecryptProviderInstanceClientSecret(instance domain.ProviderInstance) (string, error)
	LoadPublishedLinksByPostIDs(ctx context.Context, postIDs []string) (map[string]map[string]string, error)
	EnsureBootstrapAdmin(ctx context.Context, email, name, token string) error

	AdminMetrics(ctx context.Context) (domain.AdminMetrics, error)
	CreateUserAPIToken(ctx context.Context, userID, name string) (plaintext string, meta domain.APIToken, err error)
	ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error)
	RevokeUserAPIToken(ctx context.Context, userID, tokenID string) error
}

func Open(ctx context.Context, databaseURL string, encrypter *security.Encrypter) (Store, error) {
	if isPostgresURL(databaseURL) {
		return postgres.New(ctx, databaseURL, encrypter)
	}

	sqliteURL, err := normalizeSQLiteURL(databaseURL)
	if err != nil {
		return nil, err
	}
	return sqlite.New(ctx, sqliteURL, encrypter)
}

func isPostgresURL(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}

	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		return true
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "postgres", "postgresql":
		return true
	default:
		return false
	}
}

func normalizeSQLiteURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = "file:./data/goloom.db"
	}

	if value == ":memory:" {
		return value, nil
	}

	if strings.HasPrefix(strings.ToLower(value), "sqlite://") {
		trimmed := strings.TrimPrefix(value, "sqlite://")
		if trimmed == "" {
			trimmed = "./data/goloom.db"
		}
		value = "file:" + trimmed
	}

	if strings.HasPrefix(strings.ToLower(value), "sqlite:") && !strings.HasPrefix(strings.ToLower(value), "sqlite://") {
		value = "file:" + strings.TrimPrefix(value, "sqlite:")
	}

	if !strings.HasPrefix(strings.ToLower(value), "file:") {
		value = "file:" + value
	}

	if err := ensureSQLiteParentDir(value); err != nil {
		return "", err
	}
	return value, nil
}

func ensureSQLiteParentDir(dsn string) error {
	if dsn == ":memory:" {
		return nil
	}

	pathPart := strings.TrimPrefix(dsn, "file:")
	if idx := strings.Index(pathPart, "?"); idx >= 0 {
		pathPart = pathPart[:idx]
	}
	if pathPart == "" || pathPart == ":memory:" {
		return nil
	}

	cleanPath := filepath.Clean(pathPart)
	parent := filepath.Dir(cleanPath)
	if parent == "." || parent == "/" {
		return nil
	}
	return os.MkdirAll(parent, 0o755)
}
