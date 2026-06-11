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
	UpdateTeam(ctx context.Context, teamID string, input domain.UpdateTeamInput) (domain.Team, error)
	GetTeamEngagementHourHistogram(ctx context.Context, teamID string, days int) ([]domain.EngagementHourBucket, error)

	// TeamProfile methods
	CreateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error)
	GetTeamProfile(ctx context.Context, teamID string) (domain.TeamProfile, error)
	UpdateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error)
	DeleteTeamProfile(ctx context.Context, teamID string) error

	// CampaignFormat methods
	CreateCampaignFormat(ctx context.Context, teamID string, input domain.CampaignFormat) (domain.CampaignFormat, error)
	ListCampaignFormats(ctx context.Context, teamID string) ([]domain.CampaignFormat, error)
	GetCampaignFormatByID(ctx context.Context, teamID string, id string) (domain.CampaignFormat, error)
	UpdateCampaignFormat(ctx context.Context, teamID string, id string, input domain.CampaignFormat) (domain.CampaignFormat, error)
	DeleteCampaignFormat(ctx context.Context, teamID string, id string) error

	// StyleExample methods
	CreateStyleExample(ctx context.Context, teamID string, input domain.StyleExample) (domain.StyleExample, error)
	ListStyleExamples(ctx context.Context, teamID string) ([]domain.StyleExample, error)
	DeleteStyleExample(ctx context.Context, teamID string, id string) error

	// KnowledgeSource methods
	CreateKnowledgeSource(ctx context.Context, teamID string, input domain.KnowledgeSource) (domain.KnowledgeSource, error)
	ListKnowledgeSources(ctx context.Context, teamID string) ([]domain.KnowledgeSource, error)
	GetKnowledgeSourceByID(ctx context.Context, teamID string, id string) (domain.KnowledgeSource, error)
	UpdateKnowledgeSource(ctx context.Context, teamID string, id string, input domain.KnowledgeSource) (domain.KnowledgeSource, error)
	DeleteKnowledgeSource(ctx context.Context, teamID string, id string) error

	// AIJob methods
	CreateAIJob(ctx context.Context, input domain.AIJob) (domain.AIJob, error)
	GetAIJobByID(ctx context.Context, teamID string, id string) (domain.AIJob, error)
	GetAIJobByIDGlobal(ctx context.Context, id string) (domain.AIJob, error)
	ListAIJobs(ctx context.Context, teamID string, limit int) ([]domain.AIJob, error)
	UpdateAIJobStatus(ctx context.Context, id string, status domain.AIJobStatus, result []byte, errorMsg string) error
	ListPendingAIJobs(ctx context.Context, limit int) ([]domain.AIJob, error)

	// AIServiceConfig methods
	GetAIServiceConfig(ctx context.Context, teamID string) (domain.AIServiceConfig, error)
	UpsertAIServiceConfig(ctx context.Context, teamID string, input domain.AIServiceConfig) (domain.AIServiceConfig, error)

	// RSSFeedConfig methods
	CreateRSSFeedConfig(ctx context.Context, teamID string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error)
	GetRSSFeedConfigByID(ctx context.Context, teamID string, id string) (domain.RSSFeedConfig, error)
	ListRSSFeedConfigs(ctx context.Context, teamID string) ([]domain.RSSFeedConfig, error)
	UpdateRSSFeedConfig(ctx context.Context, teamID string, id string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error)
	DeleteRSSFeedConfig(ctx context.Context, teamID string, id string) error
	ListActiveRSSFeedConfigs(ctx context.Context, limit int) ([]domain.RSSFeedConfig, error)
	CountRSSFeedPostsToday(ctx context.Context, feedID string) (int, error)
	RSSItemAlreadyImported(ctx context.Context, feedID, itemKey string) (bool, error)
	RecordRSSImportedItem(ctx context.Context, feedID, itemKey, postID string) error
	UpdateRSSImportedItemPostID(ctx context.Context, feedID, itemKey, postID string) error
	IncrementRSSFeedCounter(ctx context.Context, feedID string) error
	UpdateRSSFeedLastFetched(ctx context.Context, feedID string, lastFetchedAt time.Time) error
	ListAutomationReviewDrafts(ctx context.Context, teamID string, limit int) ([]domain.ReviewQueueItem, error)

	// ProactiveTriggerSettings methods
	GetProactiveTriggerSettings(ctx context.Context, teamID string) (domain.ProactiveTriggerSettings, error)
	UpsertProactiveTriggerSettings(ctx context.Context, teamID string, input domain.ProactiveTriggerSettings) (domain.ProactiveTriggerSettings, error)

	// ExternalPostMonitor methods
	GetExternalPostMonitorSettings(ctx context.Context, teamID string) (domain.ExternalPostMonitorSettings, error)
	UpsertExternalPostMonitorSettings(ctx context.Context, teamID string, input domain.UpsertExternalPostMonitorInput) (domain.ExternalPostMonitorSettings, error)
	ListTeamsWithExternalPostMonitorEnabled(ctx context.Context, limit int) ([]domain.ExternalPostMonitorSettings, error)
	UpdateExternalPostMonitorSyncState(ctx context.Context, teamID string, lastSyncAt time.Time, backfillCompleted bool) error
	TargetExistsByRemotePostID(ctx context.Context, accountID, remotePostID string) (bool, error)
	AuthorPostAlreadyTracked(ctx context.Context, accountID, remoteID, publishedURL string, metadata map[string]string) (bool, error)
	DeleteRedundantImportedPosts(ctx context.Context, teamID string) (int, error)
	CreateImportedPost(ctx context.Context, teamID, authorUserID string, input domain.ImportedPostInput) (domain.ScheduledPost, error)

	// AI Context aggregation
	GetTeamAIContext(ctx context.Context, teamID string) (domain.AIContext, error)

	// Admin: list AI-enabled teams
	ListAIEnabledTeams(ctx context.Context) ([]domain.Team, error)
	ListDuePostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error)
	ListEnabledPostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error)
	ListPostTemplates(ctx context.Context, teamID string) ([]domain.PostTemplate, error)
	GetPostTemplate(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error)
	CreatePostTemplate(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostTemplateInput) (domain.PostTemplate, error)
	UpdatePostTemplate(ctx context.Context, teamID, templateID string, input domain.UpdatePostTemplateInput) (domain.PostTemplate, error)
	DeletePostTemplate(ctx context.Context, teamID, templateID string) error
	IsPostTemplateOccurrenceSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error)
	IsPostTemplateAnnouncementSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error)
	AddPostTemplateSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error
	AddPostTemplateAnnouncementSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error
	HasPostTemplateRoleMaterialized(ctx context.Context, templateID string, occurrenceAt time.Time, role string) (bool, error)
	ShiftPostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt, shiftTo time.Time) error
	GetPostTemplateShiftTo(ctx context.Context, templateID string, occurrenceAt time.Time) (*time.Time, error)
	AdvancePostTemplateSchedule(ctx context.Context, templateID string, nextMaterialize *time.Time, counterNext int) error
	AdvancePostTemplateAnnouncementCounter(ctx context.Context, templateID string, counterNext int) error
	ListPostTemplateLinkedPosts(ctx context.Context, teamID, templateID string) ([]domain.PostTemplateLinkedPost, error)
	DeletePostTemplateLinkedPosts(ctx context.Context, teamID, templateID string, postIDs []string) (int, error)
	SetPostTemplateMaterializationState(ctx context.Context, templateID string, nextMaterialize *time.Time, counterNext, announcementCounterNext int) error
	ListTeamMembers(ctx context.Context, teamID string) ([]domain.TeamMembership, error)
	AddTeamMember(ctx context.Context, teamID string, input domain.AddTeamMemberInput) (domain.TeamMembership, error)
	RemoveTeamMember(ctx context.Context, teamID, userID string) error
	ListProviderInstances(ctx context.Context, providerName string) ([]domain.ProviderInstance, error)
	GetProviderInstanceByID(ctx context.Context, instanceID string) (domain.ProviderInstance, error)
	CreateProviderInstance(ctx context.Context, createdByUserID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error)
	UpdateProviderInstance(ctx context.Context, instanceID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error)
	DeleteProviderInstance(ctx context.Context, instanceID string) error
	UserHasAnyTeamRole(ctx context.Context, userID, teamID string, roles ...domain.TeamRole) (bool, error)
	ListTeamAccounts(ctx context.Context, teamID string) ([]domain.SocialAccount, error)
	CreateAccount(ctx context.Context, teamID string, input domain.ConnectedAccount) (domain.SocialAccount, error)
	UpdateAccount(ctx context.Context, teamID, accountID string, input domain.UpdateAccountInput) (domain.SocialAccount, error)
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
	ListTeamPostsPage(ctx context.Context, teamID string, limit, offset int) ([]domain.ScheduledPost, int64, error)
	GetScheduledPost(ctx context.Context, teamID, postID string) (domain.ScheduledPost, error)
	GetScheduledPostByID(ctx context.Context, postID string) (domain.ScheduledPost, error)
	PatchScheduledPost(ctx context.Context, teamID, postID string, patch domain.UpdatePostPatch) (domain.ScheduledPost, error)
	CancelScheduledPost(ctx context.Context, teamID, postID string) error
	DeleteScheduledPost(ctx context.Context, teamID, postID string) error
	GetScheduledPostTemplateLink(ctx context.Context, teamID, postID string) (templateID string, occurrenceAt *time.Time, role string, err error)
	ListDuePosts(ctx context.Context, limit int) ([]domain.ScheduledPost, error)
	MarkPostProcessing(ctx context.Context, postID string) error
	MarkPostResult(ctx context.Context, postID string, attemptCount int, status domain.PostStatus, lastError string, nextAttempt *time.Time) error
	MarkPostTargetResult(ctx context.Context, postID, accountID string, status domain.PostStatus, publishedURL, lastError string, publishMetadata map[string]string, remotePostID string) error
	UpdateSocialAccountTokens(ctx context.Context, accountID string, accessToken, refreshToken string, accessExpiresAt *time.Time) error
	LoadPostTargets(ctx context.Context, postID string) ([]domain.SocialAccount, error)
	DecryptAccessToken(account domain.SocialAccount) (string, error)
	DecryptRefreshToken(account domain.SocialAccount) (string, error)
	DecryptProviderInstanceClientSecret(instance domain.ProviderInstance) (string, error)
	LoadPublishedLinksByPostIDs(ctx context.Context, postIDs []string) (map[string]map[string]string, error)
	ListPostedTargetsForMetricSync(ctx context.Context, notBefore time.Time, utcDay string, limit int) ([]domain.PostedTargetForMetricSync, error)
	UpsertPostMetrics(ctx context.Context, postID, accountID string, metrics map[string]int64, recordedAt string) error
	MarkScheduledPostTargetMetricsSynced(ctx context.Context, postID, accountID, utcDay string) error
	ListOAuthAccountsWithAccessTokenExpiringBefore(ctx context.Context, before time.Time, limit int) ([]domain.AccountOAuthTokenExpiry, error)
	ListAccountsForMetricsSync(ctx context.Context, limit int) ([]domain.SocialAccount, error)
	UpsertAccountMetrics(ctx context.Context, accountID string, metrics map[string]int64, recordedAt time.Time) error
	GetTeamAnalytics(ctx context.Context, teamID string, topPostsLimit int) (domain.TeamAnalyticsSummary, error)
	GetTeamAnalyticsReport(ctx context.Context, teamID string, topPostsLimit int) (domain.TeamAnalyticsReport, error)
	ListTeamPostAnalyticsRanking(ctx context.Context, teamID string, sort string, limit, offset int) ([]domain.PostAnalyticsListRow, error)
	GetTeamMetricHistorySeries(ctx context.Context, teamID, metric string, days int) ([]domain.MetricHistoryPoint, error)
	GetTeamAccountMetricHistorySeries(ctx context.Context, teamID, accountID string, days int) ([]domain.AccountMetricHistoryPoint, error)
	ListPostMetricsForTeamPost(ctx context.Context, teamID, postID string) ([]domain.PostMetric, error)
	ListAllPostVersionsForTeam(ctx context.Context, teamID string) ([]domain.PostVersion, error)
	ListPostVersionsForTeamPost(ctx context.Context, teamID, postID string) ([]domain.PostVersion, error)
	ApplyPostVersionsPatch(ctx context.Context, teamID, postID string, versions []domain.PostVersion) error
	EnsureBootstrapAdmin(ctx context.Context, email, name, token string) error

	CreateMediaItem(ctx context.Context, item domain.MediaItem) (domain.MediaItem, error)
	// FindMediaItemByTeamSHA256 returns ok=true when the team already ingested this file hash (dedup).
	FindMediaItemByTeamSHA256(ctx context.Context, teamID, sha256 string) (item domain.MediaItem, ok bool, err error)
	GetMediaItemByID(ctx context.Context, teamID, mediaID string) (domain.MediaItem, error)
	ListTeamMedia(ctx context.Context, teamID string) ([]domain.MediaItem, error)
	DeleteMediaItem(ctx context.Context, teamID, mediaID string) error
	GetMediaProviderMapping(ctx context.Context, mediaID, accountID string) (domain.MediaProviderMapping, error)
	UpsertMediaProviderMapping(ctx context.Context, mapping domain.MediaProviderMapping) error

	TryAcquireLock(ctx context.Context, lockID string, duration time.Duration) (bool, error)

	AdminMetrics(ctx context.Context) (domain.AdminMetrics, error)
	AdminSyncStatus(ctx context.Context, notBefore time.Time) (domain.AdminSyncStatus, error)
	FillAccountSyncTimestamps(ctx context.Context, accounts []domain.SocialAccount) error
	RepairFuturePostedPosts(ctx context.Context) (int64, error)
	CreateUserAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time, scopes string, teamID *string) (plaintext string, meta domain.APIToken, err error)
	CreateSessionAPIToken(ctx context.Context, userID string, ttl time.Duration) (plaintext string, meta domain.APIToken, err error)
	ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error)
	RevokeUserAPIToken(ctx context.Context, userID, tokenID string) error

	InsertLogEntry(ctx context.Context, e domain.LogEntry) error
	ListLogEntries(ctx context.Context, filter domain.LogFilter) ([]domain.LogEntry, error)
	CountLogEntries(ctx context.Context, filter domain.LogFilter) (int, error)
	ArchiveLogEntry(ctx context.Context, id string) error
	UnarchiveLogEntry(ctx context.Context, id string) error
	DeleteLogEntry(ctx context.Context, id string) error
	DeleteLogEntriesBefore(ctx context.Context, before time.Time) (int64, error)
}

// LogStore is the subset of Store needed for persisting and querying log entries.
// It allows the slog DBHandler to accept a minimal interface instead of the full Store.
type LogStore interface {
	InsertLogEntry(ctx context.Context, e domain.LogEntry) error
	ArchiveLogEntry(ctx context.Context, id string) error
	UnarchiveLogEntry(ctx context.Context, id string) error
	DeleteLogEntry(ctx context.Context, id string) error
	DeleteLogEntriesBefore(ctx context.Context, before time.Time) (int64, error)
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
