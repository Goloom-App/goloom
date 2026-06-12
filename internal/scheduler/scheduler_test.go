package scheduler

import (
	"context"
	"fmt"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/hashtag"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
)

// mockStore implements store.Store for scheduler tests. Unconfigured methods return safe zero values.
type mockStore struct {
	mu sync.Mutex

	listDuePostsFn func(ctx context.Context, limit int) ([]domain.ScheduledPost, error)
	listDuePosts   []domain.ScheduledPost
	listDueErr     error

	listPostVersionsForTeamPostFn  func(ctx context.Context, teamID, postID string) ([]domain.PostVersion, error)
	listPostVersionsForTeamPost    []domain.PostVersion
	listPostVersionsForTeamPostErr error

	markProcessingErr error
	markProcessingFn  func(postID string) error

	loadTargets    []domain.SocialAccount
	loadTargetsErr error

	decryptAccessFn  func(account domain.SocialAccount) (string, error)
	decryptRefreshFn func(account domain.SocialAccount) (string, error)

	markTargetCalls []markTargetCall
	markTargetErr   error

	markPostCalls []markPostCall
	markPostErr   error

	listDuePostTemplates    []domain.PostTemplate
	listDuePostTemplatesErr error

	advanceAnnouncementCounterCalls []advanceAnnouncementCall

	createScheduledPostCalls []domain.CreatePostInput
	createScheduledPostErr   error
	createScheduledPostFn    func(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error)

	listTeamMembersFn func(ctx context.Context, teamID string) ([]domain.TeamMembership, error)

	listActiveRSSFeedsFn func(ctx context.Context, limit int) ([]domain.RSSFeedConfig, error)

	isPostTemplateOccurrenceSkippedFn func(templateID string, occurrenceAt time.Time) (bool, error)
	getPostTemplateShiftToFn          func(templateID string, occurrenceAt time.Time) *time.Time
	advancePostTemplateCalls          []advanceTemplateCall

	getPostTemplateFn           func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error)
	listPostTemplateLinkedPosts []domain.PostTemplateLinkedPost
}

type advanceTemplateCall struct {
	templateID      string
	nextMaterialize *time.Time
	counterNext     int
}

type advanceAnnouncementCall struct {
	templateID  string
	counterNext int
}

type markTargetCall struct {
	postID, accountID       string
	status                  domain.PostStatus
	publishedURL, lastError string
	publishMetadata         map[string]string
}

type markPostCall struct {
	postID       string
	attemptCount int
	status       domain.PostStatus
	lastError    string
	nextAttempt  *time.Time
}

func (m *mockStore) Close() {}

func (m *mockStore) UpsertOIDCUser(ctx context.Context, subject, email, name string) (domain.User, error) {
	return domain.User{}, nil
}

func (m *mockStore) LookupAPIToken(ctx context.Context, bearerToken string) (domain.AuthenticatedPrincipal, error) {
	return domain.AuthenticatedPrincipal{}, nil
}

func (m *mockStore) ListUsers(ctx context.Context) ([]domain.User, error) { return nil, nil }

func (m *mockStore) SetUserAdmin(ctx context.Context, userID string, isAdmin bool) (domain.User, error) {
	return domain.User{}, nil
}

func (m *mockStore) ListTeamsForUser(ctx context.Context, userID string, isAdmin bool) ([]domain.Team, error) {
	return nil, nil
}

func (m *mockStore) CreateTeam(ctx context.Context, ownerUserID string, input domain.CreateTeamInput) (domain.Team, error) {
	return domain.Team{}, nil
}

func (m *mockStore) UpdateTeam(ctx context.Context, teamID string, input domain.UpdateTeamInput) (domain.Team, error) {
	return domain.Team{}, nil
}

func (m *mockStore) CreateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error) {
	return domain.TeamProfile{}, nil
}

func (m *mockStore) GetTeamProfile(ctx context.Context, teamID string) (domain.TeamProfile, error) {
	return domain.TeamProfile{}, nil
}

func (m *mockStore) UpdateTeamProfile(ctx context.Context, teamID string, input domain.TeamProfile) (domain.TeamProfile, error) {
	return domain.TeamProfile{}, nil
}

func (m *mockStore) DeleteTeamProfile(ctx context.Context, teamID string) error { return nil }

func (m *mockStore) CreateCampaignFormat(ctx context.Context, teamID string, input domain.CampaignFormat) (domain.CampaignFormat, error) {
	return domain.CampaignFormat{}, nil
}

func (m *mockStore) ListCampaignFormats(ctx context.Context, teamID string) ([]domain.CampaignFormat, error) {
	return nil, nil
}

func (m *mockStore) GetCampaignFormatByID(ctx context.Context, teamID string, id string) (domain.CampaignFormat, error) {
	return domain.CampaignFormat{}, nil
}

func (m *mockStore) UpdateCampaignFormat(ctx context.Context, teamID string, id string, input domain.CampaignFormat) (domain.CampaignFormat, error) {
	return domain.CampaignFormat{}, nil
}

func (m *mockStore) DeleteCampaignFormat(ctx context.Context, teamID string, id string) error {
	return nil
}

func (m *mockStore) CreateStyleExample(ctx context.Context, teamID string, input domain.StyleExample) (domain.StyleExample, error) {
	return domain.StyleExample{}, nil
}

func (m *mockStore) ListStyleExamples(ctx context.Context, teamID string) ([]domain.StyleExample, error) {
	return nil, nil
}

func (m *mockStore) DeleteStyleExample(ctx context.Context, teamID string, id string) error {
	return nil
}

func (m *mockStore) CreateKnowledgeSource(ctx context.Context, teamID string, input domain.KnowledgeSource) (domain.KnowledgeSource, error) {
	return domain.KnowledgeSource{}, nil
}

func (m *mockStore) ListKnowledgeSources(ctx context.Context, teamID string) ([]domain.KnowledgeSource, error) {
	return nil, nil
}

func (m *mockStore) GetKnowledgeSourceByID(ctx context.Context, teamID string, id string) (domain.KnowledgeSource, error) {
	return domain.KnowledgeSource{}, nil
}

func (m *mockStore) UpdateKnowledgeSource(ctx context.Context, teamID string, id string, input domain.KnowledgeSource) (domain.KnowledgeSource, error) {
	return domain.KnowledgeSource{}, nil
}

func (m *mockStore) DeleteKnowledgeSource(ctx context.Context, teamID string, id string) error {
	return nil
}

func (m *mockStore) CreateAIJob(ctx context.Context, input domain.AIJob) (domain.AIJob, error) {
	return domain.AIJob{}, nil
}

func (m *mockStore) GetAIJobByID(ctx context.Context, teamID string, id string) (domain.AIJob, error) {
	return domain.AIJob{}, nil
}

func (m *mockStore) GetAIJobByIDGlobal(ctx context.Context, id string) (domain.AIJob, error) {
	return domain.AIJob{}, fmt.Errorf("not implemented")
}

func (m *mockStore) ListAIJobs(ctx context.Context, teamID string, limit int) ([]domain.AIJob, error) {
	return nil, nil
}

func (m *mockStore) UpdateAIJobStatus(ctx context.Context, id string, status domain.AIJobStatus, result []byte, errorMsg string) error {
	return nil
}

func (m *mockStore) ListPendingAIJobs(ctx context.Context, limit int) ([]domain.AIJob, error) {
	return nil, nil
}

func (m *mockStore) GetAIServiceConfig(ctx context.Context, teamID string) (domain.AIServiceConfig, error) {
	return domain.AIServiceConfig{}, nil
}

func (m *mockStore) UpsertAIServiceConfig(ctx context.Context, teamID string, input domain.AIServiceConfig) (domain.AIServiceConfig, error) {
	return domain.AIServiceConfig{}, nil
}

func (m *mockStore) CreateRSSFeedConfig(ctx context.Context, teamID string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error) {
	return domain.RSSFeedConfig{}, nil
}

func (m *mockStore) GetRSSFeedConfigByID(ctx context.Context, teamID string, id string) (domain.RSSFeedConfig, error) {
	return domain.RSSFeedConfig{}, nil
}

func (m *mockStore) ListRSSFeedConfigs(ctx context.Context, teamID string) ([]domain.RSSFeedConfig, error) {
	return nil, nil
}

func (m *mockStore) UpdateRSSFeedConfig(ctx context.Context, teamID string, id string, input domain.RSSFeedConfig) (domain.RSSFeedConfig, error) {
	return domain.RSSFeedConfig{}, nil
}

func (m *mockStore) DeleteRSSFeedConfig(ctx context.Context, teamID string, id string) error {
	return nil
}

func (m *mockStore) ListActiveRSSFeedConfigs(ctx context.Context, limit int) ([]domain.RSSFeedConfig, error) {
	if m.listActiveRSSFeedsFn != nil {
		return m.listActiveRSSFeedsFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockStore) CountRSSFeedPostsToday(ctx context.Context, feedID string) (int, error) {
	return 0, nil
}

func (m *mockStore) RSSItemAlreadyImported(ctx context.Context, feedID, itemKey string) (bool, error) {
	return false, nil
}

func (m *mockStore) RecordRSSImportedItem(ctx context.Context, feedID, itemKey, postID string) error {
	return nil
}

func (m *mockStore) UpdateRSSImportedItemPostID(ctx context.Context, feedID, itemKey, postID string) error {
	return nil
}

func (m *mockStore) IncrementRSSFeedCounter(ctx context.Context, feedID string) error {
	return nil
}

func (m *mockStore) UpdateRSSFeedLastFetched(ctx context.Context, feedID string, lastFetchedAt time.Time) error {
	return nil
}

func (m *mockStore) ListAutomationReviewDrafts(ctx context.Context, teamID string, limit int) ([]domain.ReviewQueueItem, error) {
	return nil, nil
}

func (m *mockStore) GetProactiveTriggerSettings(ctx context.Context, teamID string) (domain.ProactiveTriggerSettings, error) {
	return domain.ProactiveTriggerSettings{}, nil
}

func (m *mockStore) UpsertProactiveTriggerSettings(ctx context.Context, teamID string, input domain.ProactiveTriggerSettings) (domain.ProactiveTriggerSettings, error) {
	return domain.ProactiveTriggerSettings{}, nil
}

func (m *mockStore) GetExternalPostMonitorSettings(ctx context.Context, teamID string) (domain.ExternalPostMonitorSettings, error) {
	return domain.ExternalPostMonitorSettings{TeamID: teamID}, nil
}

func (m *mockStore) UpsertExternalPostMonitorSettings(ctx context.Context, teamID string, input domain.UpsertExternalPostMonitorInput) (domain.ExternalPostMonitorSettings, error) {
	return domain.ExternalPostMonitorSettings{TeamID: teamID, Enabled: input.Enabled}, nil
}

func (m *mockStore) ListTeamsWithExternalPostMonitorEnabled(ctx context.Context, limit int) ([]domain.ExternalPostMonitorSettings, error) {
	return nil, nil
}

func (m *mockStore) UpdateExternalPostMonitorSyncState(ctx context.Context, teamID string, lastSyncAt time.Time, backfillCompleted bool) error {
	return nil
}

func (m *mockStore) TargetExistsByRemotePostID(ctx context.Context, accountID, remotePostID string) (bool, error) {
	return false, nil
}

func (m *mockStore) AuthorPostAlreadyTracked(ctx context.Context, accountID, remoteID, publishedURL string, metadata map[string]string) (bool, error) {
	return false, nil
}

func (m *mockStore) DeleteRedundantImportedPosts(ctx context.Context, teamID string) (int, error) {
	return 0, nil
}

func (m *mockStore) CreateImportedPost(ctx context.Context, teamID, authorUserID string, input domain.ImportedPostInput) (domain.ScheduledPost, error) {
	return domain.ScheduledPost{}, nil
}

func (m *mockStore) GetTeamAIContext(ctx context.Context, teamID string) (domain.AIContext, error) {
	return domain.AIContext{}, nil
}

func (m *mockStore) ListAIEnabledTeams(ctx context.Context) ([]domain.Team, error) { return nil, nil }
func (m *mockStore) ListTeamMembers(ctx context.Context, teamID string) ([]domain.TeamMembership, error) {
	if m.listTeamMembersFn != nil {
		return m.listTeamMembersFn(ctx, teamID)
	}
	return nil, nil
}

func (m *mockStore) AddTeamMember(ctx context.Context, teamID string, input domain.AddTeamMemberInput) (domain.TeamMembership, error) {
	return domain.TeamMembership{}, nil
}

func (m *mockStore) RemoveTeamMember(ctx context.Context, teamID, userID string) error { return nil }

func (m *mockStore) ListProviderInstances(ctx context.Context, providerName string) ([]domain.ProviderInstance, error) {
	return nil, nil
}

func (m *mockStore) GetProviderInstanceByID(ctx context.Context, instanceID string) (domain.ProviderInstance, error) {
	return domain.ProviderInstance{}, nil
}

func (m *mockStore) CreateProviderInstance(ctx context.Context, createdByUserID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error) {
	return domain.ProviderInstance{}, nil
}

func (m *mockStore) UpdateProviderInstance(ctx context.Context, instanceID string, input domain.PreparedProviderInstance) (domain.ProviderInstance, error) {
	return domain.ProviderInstance{}, nil
}

func (m *mockStore) DeleteProviderInstance(ctx context.Context, instanceID string) error {
	return nil
}

func (m *mockStore) UserHasAnyTeamRole(ctx context.Context, userID, teamID string, roles ...domain.TeamRole) (bool, error) {
	return false, nil
}

func (m *mockStore) ListTeamAccounts(ctx context.Context, teamID string) ([]domain.SocialAccount, error) {
	return nil, nil
}

func (m *mockStore) CreateAccount(ctx context.Context, teamID string, input domain.ConnectedAccount) (domain.SocialAccount, error) {
	return domain.SocialAccount{}, nil
}

func (m *mockStore) DeleteAccount(ctx context.Context, teamID, accountID string) error { return nil }

func (m *mockStore) UpdateAccount(ctx context.Context, teamID, accountID string, input domain.UpdateAccountInput) (domain.SocialAccount, error) {
	return domain.SocialAccount{}, nil
}

func (m *mockStore) GetAccountsByIDs(ctx context.Context, teamID string, ids []string) ([]domain.SocialAccount, error) {
	return nil, nil
}

func (m *mockStore) CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createScheduledPostCalls = append(m.createScheduledPostCalls, input)
	if m.createScheduledPostFn != nil {
		return m.createScheduledPostFn(ctx, teamID, principal, input)
	}
	return domain.ScheduledPost{}, m.createScheduledPostErr
}

func (m *mockStore) ListTeamPosts(ctx context.Context, teamID string) ([]domain.ScheduledPost, error) {
	return nil, nil
}

func (m *mockStore) GetScheduledPost(ctx context.Context, teamID, postID string) (domain.ScheduledPost, error) {
	return domain.ScheduledPost{}, nil
}

func (m *mockStore) PatchScheduledPost(ctx context.Context, teamID, postID string, patch domain.UpdatePostPatch) (domain.ScheduledPost, error) {
	return domain.ScheduledPost{}, nil
}

func (m *mockStore) CancelScheduledPost(ctx context.Context, teamID, postID string) error { return nil }

func (m *mockStore) DeleteScheduledPost(ctx context.Context, teamID, postID string) error { return nil }

func (m *mockStore) ListDuePosts(ctx context.Context, limit int) ([]domain.ScheduledPost, error) {
	m.mu.Lock()
	fn := m.listDuePostsFn
	posts := m.listDuePosts
	err := m.listDueErr
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, limit)
	}
	return posts, err
}

func (m *mockStore) MarkPostProcessing(ctx context.Context, postID string) error {
	m.mu.Lock()
	fn := m.markProcessingFn
	errStatic := m.markProcessingErr
	m.mu.Unlock()
	if fn != nil {
		return fn(postID)
	}
	return errStatic
}

func (m *mockStore) MarkPostResult(ctx context.Context, postID string, attemptCount int, status domain.PostStatus, lastError string, nextAttempt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markPostErr != nil {
		return m.markPostErr
	}
	m.markPostCalls = append(m.markPostCalls, markPostCall{
		postID: postID, attemptCount: attemptCount, status: status,
		lastError: lastError, nextAttempt: nextAttempt,
	})
	return nil
}

func (m *mockStore) MarkPostTargetResult(ctx context.Context, postID, accountID string, status domain.PostStatus, publishedURL, lastError string, publishMetadata map[string]string, remotePostID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markTargetErr != nil {
		return m.markTargetErr
	}
	m.markTargetCalls = append(m.markTargetCalls, markTargetCall{
		postID: postID, accountID: accountID, status: status,
		publishedURL: publishedURL, lastError: lastError, publishMetadata: publishMetadata,
	})
	return nil
}

func (m *mockStore) UpdateSocialAccountTokens(ctx context.Context, accountID string, accessToken, refreshToken string, accessExpiresAt *time.Time) error {
	return nil
}

func (m *mockStore) LoadPostTargets(ctx context.Context, postID string) ([]domain.SocialAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadTargets, m.loadTargetsErr
}

func (m *mockStore) DecryptAccessToken(account domain.SocialAccount) (string, error) {
	m.mu.Lock()
	fn := m.decryptAccessFn
	m.mu.Unlock()
	if fn != nil {
		return fn(account)
	}
	return "access-plain", nil
}

func (m *mockStore) DecryptRefreshToken(account domain.SocialAccount) (string, error) {
	m.mu.Lock()
	fn := m.decryptRefreshFn
	m.mu.Unlock()
	if fn != nil {
		return fn(account)
	}
	return "refresh-plain", nil
}

func (m *mockStore) DecryptProviderInstanceClientSecret(instance domain.ProviderInstance) (string, error) {
	return "", nil
}

func (m *mockStore) LoadPublishedLinksByPostIDs(ctx context.Context, postIDs []string) (map[string]map[string]string, error) {
	return map[string]map[string]string{}, nil
}

func (m *mockStore) ListPostedTargetsForMetricSync(_ context.Context, _ time.Time, _ string, _ int) ([]domain.PostedTargetForMetricSync, error) {
	return nil, nil
}

func (m *mockStore) UpsertPostMetrics(ctx context.Context, postID, accountID string, metrics map[string]int64, recordedAt string) error {
	return nil
}

func (m *mockStore) MarkScheduledPostTargetMetricsSynced(ctx context.Context, postID, accountID, utcDay string) error {
	return nil
}

func (m *mockStore) ListOAuthAccountsWithAccessTokenExpiringBefore(ctx context.Context, before time.Time, limit int) ([]domain.AccountOAuthTokenExpiry, error) {
	return nil, nil
}

func (m *mockStore) ListAccountsForMetricsSync(ctx context.Context, limit int) ([]domain.SocialAccount, error) {
	return nil, nil
}

func (m *mockStore) UpsertAccountMetrics(ctx context.Context, accountID string, metrics map[string]int64, recordedAt time.Time) error {
	return nil
}

func (m *mockStore) GetTeamAnalytics(ctx context.Context, teamID string, topPostsLimit int) (domain.TeamAnalyticsSummary, error) {
	return domain.TeamAnalyticsSummary{MetricsTotal: map[string]int64{}, TopPosts: nil}, nil
}

func (m *mockStore) GetTeamAnalyticsReport(ctx context.Context, teamID string, topPostsLimit int) (domain.TeamAnalyticsReport, error) {
	return domain.TeamAnalyticsReport{}, nil
}

func (m *mockStore) ListTeamPostAnalyticsRanking(ctx context.Context, teamID string, sort string, limit, offset int) ([]domain.PostAnalyticsListRow, error) {
	return nil, nil
}

func (m *mockStore) GetTeamMetricHistorySeries(ctx context.Context, teamID, metric string, days int) ([]domain.MetricHistoryPoint, error) {
	return nil, nil
}

func (m *mockStore) GetTeamAccountMetricHistorySeries(ctx context.Context, teamID, accountID string, days int) ([]domain.AccountMetricHistoryPoint, error) {
	return nil, nil
}

func (m *mockStore) ListPostMetricsForTeamPost(ctx context.Context, teamID, postID string) ([]domain.PostMetric, error) {
	return nil, nil
}

func (m *mockStore) ListPostVersionsForTeamPost(ctx context.Context, teamID, postID string) ([]domain.PostVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listPostVersionsForTeamPostFn != nil {
		return m.listPostVersionsForTeamPostFn(ctx, teamID, postID)
	}
	if m.listPostVersionsForTeamPostErr != nil {
		return nil, m.listPostVersionsForTeamPostErr
	}
	return m.listPostVersionsForTeamPost, nil
}

func (m *mockStore) ListAllPostVersionsForTeam(ctx context.Context, teamID string) ([]domain.PostVersion, error) {
	return nil, nil
}

func (m *mockStore) ApplyPostVersionsPatch(ctx context.Context, teamID, postID string, versions []domain.PostVersion) error {
	return nil
}

func (m *mockStore) EnsureBootstrapAdmin(ctx context.Context, email, name, token string) error {
	return nil
}

func (m *mockStore) EnsurePersonalTeam(ctx context.Context, userID string) (domain.Team, error) {
	return domain.Team{}, nil
}

func (m *mockStore) EnsurePersonalTeamsMigrated(ctx context.Context) error { return nil }

func (m *mockStore) GetTeamByID(ctx context.Context, teamID string) (domain.Team, error) {
	return domain.Team{}, nil
}

func (m *mockStore) DeleteSocialAccount(ctx context.Context, accountID string) error { return nil }

func (m *mockStore) GetAccountByID(ctx context.Context, accountID string) (domain.SocialAccount, error) {
	return domain.SocialAccount{}, nil
}

func (m *mockStore) GetAccountsByIDsGlobal(ctx context.Context, ids []string) ([]domain.SocialAccount, error) {
	return nil, nil
}

func (m *mockStore) GetScheduledPostByID(ctx context.Context, postID string) (domain.ScheduledPost, error) {
	return domain.ScheduledPost{}, nil
}

func (m *mockStore) MigrateAccountToTeam(ctx context.Context, userID string, accountID, targetTeamID string, isAdmin bool) error {
	return nil
}

func (m *mockStore) CreateTeamInvitation(ctx context.Context, teamID, createdByUserID string, input domain.CreateTeamInvitationInput) (domain.TeamInvitation, string, error) {
	return domain.TeamInvitation{}, "", nil
}

func (m *mockStore) AcceptTeamInvitation(ctx context.Context, userID, email, rawToken string) (domain.TeamMembership, error) {
	return domain.TeamMembership{}, nil
}

func (m *mockStore) AdminMetrics(ctx context.Context) (domain.AdminMetrics, error) {
	return domain.AdminMetrics{}, nil
}

func (m *mockStore) AdminSyncStatus(ctx context.Context, notBefore time.Time) (domain.AdminSyncStatus, error) {
	return domain.AdminSyncStatus{}, nil
}

func (m *mockStore) FillAccountSyncTimestamps(ctx context.Context, accounts []domain.SocialAccount) error {
	return nil
}

func (m *mockStore) RepairFuturePostedPosts(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockStore) CreateUserAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time, scopes string, teamID *string) (string, domain.APIToken, error) {
	return "", domain.APIToken{}, nil
}

func (m *mockStore) CreateSessionAPIToken(ctx context.Context, userID string, ttl time.Duration) (string, domain.APIToken, error) {
	return "", domain.APIToken{}, nil
}

func (m *mockStore) ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	return nil, nil
}

func (m *mockStore) RevokeUserAPIToken(ctx context.Context, userID, tokenID string) error {
	return nil
}

func (m *mockStore) CreateMediaItem(ctx context.Context, item domain.MediaItem) (domain.MediaItem, error) {
	return domain.MediaItem{}, errors.New("mock: CreateMediaItem not configured")
}

func (m *mockStore) FindMediaItemByTeamSHA256(ctx context.Context, teamID, sha256 string) (domain.MediaItem, bool, error) {
	return domain.MediaItem{}, false, nil
}

func (m *mockStore) GetMediaItemByID(ctx context.Context, teamID, mediaID string) (domain.MediaItem, error) {
	return domain.MediaItem{}, errors.New("mock: GetMediaItemByID not configured")
}

func (m *mockStore) ListTeamMedia(ctx context.Context, teamID string) ([]domain.MediaItem, error) {
	return nil, nil
}

func (m *mockStore) DeleteMediaItem(ctx context.Context, teamID, mediaID string) error { return nil }

func (m *mockStore) GetMediaProviderMapping(ctx context.Context, mediaID, accountID string) (domain.MediaProviderMapping, error) {
	return domain.MediaProviderMapping{}, errors.New("mock: no mapping")
}

func (m *mockStore) UpsertMediaProviderMapping(ctx context.Context, mapping domain.MediaProviderMapping) error {
	return nil
}

func (m *mockStore) GetTeamEngagementHourHistogram(ctx context.Context, teamID string, days int) ([]domain.EngagementHourBucket, error) {
	return nil, nil
}

func (m *mockStore) GetTeamEngagementHeatmap(ctx context.Context, teamID string, days int) ([]domain.EngagementHeatmapBucket, error) {
	return nil, nil
}

func (m *mockStore) ReplacePostHashtags(ctx context.Context, postID, accountID string, tags []hashtag.Tag) error {
	return nil
}

func (m *mockStore) BackfillPostHashtags(ctx context.Context) error {
	return nil
}

func (m *mockStore) ListTeamHashtagPerformance(ctx context.Context, teamID string, days int, provider string, limit int) ([]domain.HashtagPerformance, error) {
	return nil, nil
}

func (m *mockStore) GetTeamHashtagInsights(ctx context.Context, teamID string, days int, provider string) (domain.HashtagInsights, error) {
	return domain.HashtagInsights{}, nil
}

func (m *mockStore) AdvancePostTemplateAnnouncementCounter(ctx context.Context, templateID string, counterNext int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advanceAnnouncementCounterCalls = append(m.advanceAnnouncementCounterCalls, advanceAnnouncementCall{
		templateID:  templateID,
		counterNext: counterNext,
	})
	return nil
}

func (m *mockStore) ListPostTemplateLinkedPosts(ctx context.Context, teamID, templateID string) ([]domain.PostTemplateLinkedPost, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listPostTemplateLinkedPosts != nil {
		return append([]domain.PostTemplateLinkedPost(nil), m.listPostTemplateLinkedPosts...), nil
	}
	return nil, nil
}

func (m *mockStore) DeletePostTemplateLinkedPosts(ctx context.Context, teamID, templateID string, postIDs []string) (int, error) {
	return len(postIDs), nil
}

func (m *mockStore) SetPostTemplateMaterializationState(ctx context.Context, templateID string, nextMaterialize *time.Time, counterNext, announcementCounterNext int) error {
	return nil
}

func (m *mockStore) ListDuePostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error) {
	return m.ListEnabledPostTemplates(ctx, limit)
}

func (m *mockStore) ListEnabledPostTemplates(ctx context.Context, limit int) ([]domain.PostTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listDuePostTemplates, m.listDuePostTemplatesErr
}

func (m *mockStore) IsPostTemplateAnnouncementSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error) {
	return false, nil
}

func (m *mockStore) AddPostTemplateAnnouncementSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error {
	return nil
}

func (m *mockStore) HasPostTemplateRoleMaterialized(ctx context.Context, templateID string, occurrenceAt time.Time, role string) (bool, error) {
	return false, nil
}

func (m *mockStore) GetScheduledPostTemplateLink(ctx context.Context, teamID, postID string) (string, *time.Time, string, error) {
	return "", nil, "", nil
}

func (m *mockStore) ListPostTemplates(ctx context.Context, teamID string) ([]domain.PostTemplate, error) {
	return nil, nil
}

func (m *mockStore) ListTeamPostsPage(ctx context.Context, teamID string, limit, offset int) ([]domain.ScheduledPost, int64, error) {
	return nil, 0, nil
}

func (m *mockStore) GetPostTemplate(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
	m.mu.Lock()
	fn := m.getPostTemplateFn
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, teamID, templateID)
	}
	return domain.PostTemplate{}, nil
}

func (m *mockStore) CreatePostTemplate(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostTemplateInput) (domain.PostTemplate, error) {
	return domain.PostTemplate{}, nil
}

func (m *mockStore) UpdatePostTemplate(ctx context.Context, teamID, templateID string, input domain.UpdatePostTemplateInput) (domain.PostTemplate, error) {
	return domain.PostTemplate{}, nil
}

func (m *mockStore) DeletePostTemplate(ctx context.Context, teamID, templateID string) error {
	return nil
}

func (m *mockStore) IsPostTemplateOccurrenceSkipped(ctx context.Context, templateID string, occurrenceAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isPostTemplateOccurrenceSkippedFn != nil {
		return m.isPostTemplateOccurrenceSkippedFn(templateID, occurrenceAt)
	}
	return false, nil
}

func (m *mockStore) AddPostTemplateSkip(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) error {
	return nil
}

func (m *mockStore) ShiftPostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt, shiftTo time.Time) error {
	return nil
}

func (m *mockStore) GetPostTemplateShiftTo(ctx context.Context, templateID string, occurrenceAt time.Time) (*time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getPostTemplateShiftToFn != nil {
		return m.getPostTemplateShiftToFn(templateID, occurrenceAt), nil
	}
	return nil, nil
}

func (m *mockStore) AdvancePostTemplateSchedule(ctx context.Context, templateID string, nextMaterialize *time.Time, counterNext int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advancePostTemplateCalls = append(m.advancePostTemplateCalls, advanceTemplateCall{
		templateID:      templateID,
		nextMaterialize: nextMaterialize,
		counterNext:     counterNext,
	})
	return nil
}

func (m *mockStore) TryAcquireLock(ctx context.Context, lockID string, duration time.Duration) (bool, error) {
	return true, nil
}

func (m *mockStore) InsertLogEntry(ctx context.Context, e domain.LogEntry) error { return nil }
func (m *mockStore) ListLogEntries(ctx context.Context, filter domain.LogFilter) ([]domain.LogEntry, error) {
	return nil, nil
}
func (m *mockStore) CountLogEntries(ctx context.Context, filter domain.LogFilter) (int, error) {
	return 0, nil
}
func (m *mockStore) ArchiveLogEntry(ctx context.Context, id string) error   { return nil }
func (m *mockStore) UnarchiveLogEntry(ctx context.Context, id string) error { return nil }
func (m *mockStore) DeleteLogEntry(ctx context.Context, id string) error    { return nil }
func (m *mockStore) DeleteLogEntriesBefore(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

var _ store.Store = (*mockStore)(nil)

type fakeProvider struct {
	name       string
	publishRes provider.PublishResult
	publishErr error
	pubMu      sync.Mutex
	published  []provider.PublishRequest
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Capabilities(ctx context.Context, account domain.SocialAccount) (provider.Capabilities, error) {
	return provider.Capabilities{MaxChars: 500}, nil
}

func (f *fakeProvider) PrepareProviderInstance(ctx context.Context, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	return domain.PreparedProviderInstance{}, nil
}

func (f *fakeProvider) ConnectAccount(ctx context.Context, input domain.CreateAccountInput, instance *domain.ProviderInstance) (domain.ConnectedAccount, error) {
	return domain.ConnectedAccount{}, nil
}

func (f *fakeProvider) UploadMedia(ctx context.Context, account domain.SocialAccount, auth provider.PublishAuth, file io.Reader, filename, mimeType, altText string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeProvider) Publish(ctx context.Context, account domain.SocialAccount, auth provider.PublishAuth, req provider.PublishRequest) (provider.PublishResult, error) {
	f.pubMu.Lock()
	f.published = append(f.published, req)
	f.pubMu.Unlock()
	if f.publishErr != nil {
		return provider.PublishResult{}, f.publishErr
	}
	return f.publishRes, nil
}

func (f *fakeProvider) GetMetrics(ctx context.Context, account domain.SocialAccount, auth provider.PublishAuth, publishedURL string) ([]provider.EngagementMetric, error) {
	return nil, nil
}

func (f *fakeProvider) GetAccountMetrics(ctx context.Context, account domain.SocialAccount, auth provider.PublishAuth) ([]provider.AccountMetric, error) {
	return nil, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew_workersDefault(t *testing.T) {
	t.Parallel()
	st := &mockStore{}
	reg := provider.NewRegistry()
	svc := New(testLogger(), st, reg, time.Minute, 0, 0, 0, 0, 0, nil)
	if svc.workers != 1 {
		t.Fatalf("workers want 1, got %d", svc.workers)
	}
	svcNeg := New(testLogger(), st, reg, time.Minute, -3, 0, 0, 0, 0, nil)
	if svcNeg.workers != 1 {
		t.Fatalf("negative workers want 1, got %d", svcNeg.workers)
	}
}

func TestService_enqueueDuePosts_listsMarksAndEnqueues(t *testing.T) {
	st := &mockStore{
		listDuePosts: []domain.ScheduledPost{
			{ID: "p1", Content: "c1"},
			{ID: "p2", Content: "c2"},
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 2, 0, 0, 0, 0, nil)
	ctx := context.Background()
	q := make(chan domain.ScheduledPost, 4)
	if err := svc.enqueueDuePosts(ctx, q); err != nil {
		t.Fatal(err)
	}
	close(q)
	var ids []string
	for p := range q {
		ids = append(ids, p.ID)
	}
	if len(ids) != 2 || ids[0] != "p1" || ids[1] != "p2" {
		t.Fatalf("queue: %#v", ids)
	}
}

func TestService_enqueueDuePosts_listError(t *testing.T) {
	want := context.Canceled
	st := &mockStore{listDueErr: want}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	q := make(chan domain.ScheduledPost, 1)
	err := svc.enqueueDuePosts(context.Background(), q)
	if err != want {
		t.Fatalf("err: %v", err)
	}
}

func TestService_enqueueDuePosts_markProcessingError_skipsPost(t *testing.T) {
	st := &mockStore{
		listDuePosts: []domain.ScheduledPost{{ID: "bad"}, {ID: "ok"}},
		markProcessingFn: func(postID string) error {
			if postID == "bad" {
				return errMarkFail{}
			}
			return nil
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	q := make(chan domain.ScheduledPost, 4)
	if err := svc.enqueueDuePosts(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	close(q)
	var ids []string
	for p := range q {
		ids = append(ids, p.ID)
	}
	// First mark fails -> skip that post; second succeeds
	if len(ids) != 1 || ids[0] != "ok" {
		t.Fatalf("expected only ok post, got %#v", ids)
	}
}

type errMarkFail struct{}

func (errMarkFail) Error() string { return "mark failed" }

func TestService_processPost_noTargets_marksPosted(t *testing.T) {
	reg := provider.NewRegistry(&fakeProvider{name: "mastodon"})
	st := &mockStore{loadTargets: nil}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "solo", Content: "x", AttemptCount: 0})
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markPostCalls) != 1 || st.markPostCalls[0].status != domain.PostStatusPosted {
		t.Fatalf("expected posted with no targets: %#v", st.markPostCalls)
	}
	if len(st.markTargetCalls) != 0 {
		t.Fatalf("unexpected target marks: %#v", st.markTargetCalls)
	}
}

func TestService_processPost_firstTargetFailsSecondSucceeds(t *testing.T) {
	ok := &fakeProvider{name: "ok", publishRes: provider.PublishResult{URL: "https://ok", RemoteID: "1"}}
	reg := provider.NewRegistry(ok)
	st := &mockStore{
		loadTargets: []domain.SocialAccount{
			{ID: "bad", Provider: "missing"},
			{ID: "good", Provider: "ok"},
		},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "p1", Content: "c", AttemptCount: 0})
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markTargetCalls) != 2 {
		t.Fatalf("want 2 target updates, got %#v", st.markTargetCalls)
	}
	if st.markPostCalls[len(st.markPostCalls)-1].nextAttempt == nil {
		t.Fatal("expected failPost retry (nextAttempt set)")
	}
}

func TestService_processPost_success(t *testing.T) {
	fp := &fakeProvider{name: "mastodon", publishRes: provider.PublishResult{URL: "https://ex/u", RemoteID: "1"}}
	reg := provider.NewRegistry(fp)
	st := &mockStore{
		loadTargets: []domain.SocialAccount{
			{ID: "a1", Provider: "mastodon"},
		},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	post := domain.ScheduledPost{ID: "post1", TeamID: "team1", Content: "hi", AttemptCount: 0}
	svc.processPost(context.Background(), post)

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markTargetCalls) != 1 || st.markTargetCalls[0].status != domain.PostStatusPosted {
		t.Fatalf("markTargetCalls: %#v", st.markTargetCalls)
	}
	if len(st.markPostCalls) != 1 {
		t.Fatalf("markPostCalls: %#v", st.markPostCalls)
	}
	last := st.markPostCalls[len(st.markPostCalls)-1]
	if last.status != domain.PostStatusPosted || last.attemptCount != 1 || last.nextAttempt != nil {
		t.Fatalf("unexpected mark post: %#v", last)
	}
}

func TestService_processPost_appliesPostVersionPerAccount(t *testing.T) {
	fp := &fakeProvider{name: "mastodon", publishRes: provider.PublishResult{URL: "https://ex/u", RemoteID: "1"}}
	reg := provider.NewRegistry(fp)
	st := &mockStore{
		loadTargets: []domain.SocialAccount{
			{ID: "a1", Provider: "mastodon"},
			{ID: "a2", Provider: "mastodon"},
		},
		listPostVersionsForTeamPost: []domain.PostVersion{
			{PostID: "post1", AccountID: "a1", Content: "only a1"},
		},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	post := domain.ScheduledPost{ID: "post1", TeamID: "t1", Content: "default", AttemptCount: 0}
	svc.processPost(context.Background(), post)

	fp.pubMu.Lock()
	defer fp.pubMu.Unlock()
	if len(fp.published) != 2 {
		t.Fatalf("want 2 publishes, got %d", len(fp.published))
	}
	if fp.published[0].Content != "only a1" {
		t.Fatalf("a1 content: %q", fp.published[0].Content)
	}
	if fp.published[1].Content != "default" {
		t.Fatalf("a2 content: %q", fp.published[1].Content)
	}
}

type errListVersions struct{}

func (errListVersions) Error() string { return "list versions failed" }

func TestService_processPost_listVersionsError_schedulesRetry(t *testing.T) {
	fp := &fakeProvider{name: "mastodon", publishRes: provider.PublishResult{URL: "x", RemoteID: "1"}}
	reg := provider.NewRegistry(fp)
	st := &mockStore{
		loadTargets:                    []domain.SocialAccount{{ID: "a1", Provider: "mastodon"}},
		listPostVersionsForTeamPostErr: errListVersions{},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "p1", TeamID: "t1", Content: "c", AttemptCount: 0})

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markTargetCalls) != 0 {
		t.Fatalf("expected no target marks, got %#v", st.markTargetCalls)
	}
	if len(st.markPostCalls) != 1 || st.markPostCalls[0].nextAttempt == nil {
		t.Fatalf("expected retry mark post, got %#v", st.markPostCalls)
	}
}

func TestService_processPost_unsupportedProvider(t *testing.T) {
	reg := provider.NewRegistry()
	st := &mockStore{
		loadTargets: []domain.SocialAccount{{ID: "a1", Provider: "unknown"}},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "p1", AttemptCount: 0})

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markTargetCalls) != 1 || st.markTargetCalls[0].status != domain.PostStatusFailed {
		t.Fatalf("target calls %#v", st.markTargetCalls)
	}
	if len(st.markPostCalls) != 1 || st.markPostCalls[0].nextAttempt == nil {
		t.Fatalf("expected retry schedule, got %#v", st.markPostCalls)
	}
}

func TestService_processPost_decryptAccessError(t *testing.T) {
	fp := &fakeProvider{name: "mastodon"}
	reg := provider.NewRegistry(fp)
	st := &mockStore{
		loadTargets: []domain.SocialAccount{{ID: "a1", Provider: "mastodon"}},
		decryptAccessFn: func(account domain.SocialAccount) (string, error) {
			return "", errDecrypt{}
		},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "p1", AttemptCount: 0})
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markTargetCalls) != 1 || st.markTargetCalls[0].status != domain.PostStatusFailed {
		t.Fatalf("calls %#v", st.markTargetCalls)
	}
}

type errDecrypt struct{}

func (errDecrypt) Error() string { return "decrypt" }

func TestService_processPost_decryptRefreshError(t *testing.T) {
	fp := &fakeProvider{name: "mastodon"}
	reg := provider.NewRegistry(fp)
	st := &mockStore{
		loadTargets: []domain.SocialAccount{{ID: "a1", Provider: "mastodon"}},
		decryptRefreshFn: func(account domain.SocialAccount) (string, error) {
			return "", errDecrypt{}
		},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "p1", AttemptCount: 0})
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markTargetCalls) != 1 || st.markTargetCalls[0].status != domain.PostStatusFailed {
		t.Fatalf("calls %#v", st.markTargetCalls)
	}
}

func TestService_processPost_publishError(t *testing.T) {
	fp := &fakeProvider{name: "mastodon", publishErr: errPub{}}
	reg := provider.NewRegistry(fp)
	st := &mockStore{
		loadTargets: []domain.SocialAccount{{ID: "a1", Provider: "mastodon"}},
	}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "p1", AttemptCount: 0})
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markTargetCalls) != 1 || st.markTargetCalls[0].status != domain.PostStatusFailed {
		t.Fatalf("calls %#v", st.markTargetCalls)
	}
}

type errPub struct{}

func (errPub) Error() string { return "publish failed" }

func TestService_processPost_loadTargetsError(t *testing.T) {
	st := &mockStore{loadTargetsErr: errLoad{}}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(context.Background(), domain.ScheduledPost{ID: "p1", AttemptCount: 0})
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markPostCalls) != 1 || st.markPostCalls[0].status != domain.PostStatusFailed {
		t.Fatalf("markPostCalls %#v", st.markPostCalls)
	}
}

type errLoad struct{}

func (errLoad) Error() string { return "load" }

func TestService_failPost_finalFailureNoNextAttempt(t *testing.T) {
	st := &mockStore{}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	post := domain.ScheduledPost{ID: "p1", AttemptCount: 4} // +1 => 5
	svc.failPost(context.Background(), post, errPub{})

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markPostCalls) != 1 {
		t.Fatalf("calls %#v", st.markPostCalls)
	}
	c := st.markPostCalls[0]
	if c.attemptCount != 5 || c.status != domain.PostStatusFailed || c.nextAttempt != nil {
		t.Fatalf("unexpected %#v", c)
	}
}

func TestService_failPost_retrySchedulesNextAttempt(t *testing.T) {
	st := &mockStore{}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	post := domain.ScheduledPost{ID: "p1", AttemptCount: 1} // next attempt count 2
	before := time.Now()
	svc.failPost(context.Background(), post, errPub{})

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.markPostCalls) != 1 {
		t.Fatalf("calls %#v", st.markPostCalls)
	}
	c := st.markPostCalls[0]
	if c.nextAttempt == nil {
		t.Fatal("expected nextAttempt")
	}
	// attemptCount was 1 -> new count 2 -> delay 2*2 = 4 minutes
	wantMin := before.Add(4*time.Minute - time.Second)
	wantMax := before.Add(4*time.Minute + 5*time.Second)
	if c.nextAttempt.Before(wantMin) || c.nextAttempt.After(wantMax) {
		t.Fatalf("nextAttempt %v not near 4m from %v", c.nextAttempt, before)
	}
}

func TestService_Start_stopsOnContextCancel(t *testing.T) {
	st := &mockStore{listDuePosts: nil}
	svc := New(testLogger(), st, provider.NewRegistry(), 20*time.Millisecond, 1, 0, 0, 0, 0, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.Start(ctx)
		close(done)
	}()
	time.Sleep(45 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestService_materializePostTemplates_shift(t *testing.T) {
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	shiftTo := time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC)
	tmpl := domain.PostTemplate{
		ID:                "tmpl1",
		TeamID:            "team1",
		AuthorUserID:      "user1",
		Title:             "shift-test",
		Content:           "shifted post {counter}",
		RecurrenceJSON:    `{"kind":"weekly","weekdays":[3],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:  []string{"acc1"},
		Enabled:           true,
		NextMaterializeAt: &now,
		CounterNext:       3,
	}

	st := &mockStore{
		listDuePostTemplates: []domain.PostTemplate{tmpl},
		isPostTemplateOccurrenceSkippedFn: func(templateID string, occurrenceAt time.Time) (bool, error) {
			return true, nil
		},
		getPostTemplateShiftToFn: func(templateID string, occurrenceAt time.Time) *time.Time {
			return &shiftTo
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	err := svc.materializePostTemplates(context.Background())
	if err != nil {
		t.Fatalf("materializePostTemplates: %v", err)
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.createScheduledPostCalls) != 1 {
		t.Fatalf("expected 1 createScheduledPost call, got %d", len(st.createScheduledPostCalls))
	}
	if !st.createScheduledPostCalls[0].ScheduledAt.Equal(shiftTo) {
		t.Fatalf("expected ScheduledAt=%v, got %v", shiftTo, st.createScheduledPostCalls[0].ScheduledAt)
	}
	if len(st.advancePostTemplateCalls) != 1 {
		t.Fatalf("expected 1 advancePostTemplate call, got %d", len(st.advancePostTemplateCalls))
	}
	if st.advancePostTemplateCalls[0].counterNext != 4 {
		t.Fatalf("expected counterNext=4 (shifted+1), got %d", st.advancePostTemplateCalls[0].counterNext)
	}
}

func TestService_materializePostTemplates_announcement(t *testing.T) {
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	parent := domain.PostTemplate{
		ID:                     "parent1",
		TeamID:                 "team1",
		AuthorUserID:           "user1",
		Title:                  "main event",
		Content:                "main post {counter}",
		RecurrenceJSON:         `{"kind":"weekly","weekdays":[3],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:       []string{"acc1"},
		Enabled:                true,
		NextMaterializeAt:      &now,
		CounterNext:            7,
		AnnouncementEnabled:    true,
		AnnouncementTitle:      "announcement",
		AnnouncementContent:    "episode #{main_counter} on {main_month}/{main_day} ({main_weekday_name})",
		AnnouncementDaysBefore: 2,
		AnnouncementCounterNext: 1,
	}

	st := &mockStore{
		listDuePostTemplates: []domain.PostTemplate{parent},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	err := svc.materializePostTemplates(context.Background())
	if err != nil {
		t.Fatalf("materializePostTemplates: %v", err)
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.createScheduledPostCalls) != 2 {
		t.Fatalf("expected 2 createScheduledPost calls (parent + announcement), got %d", len(st.createScheduledPostCalls))
	}
	// First call: parent post at `now`
	parentCall := st.createScheduledPostCalls[0]
	if !parentCall.ScheduledAt.Equal(now) {
		t.Fatalf("parent ScheduledAt: want %v, got %v", now, parentCall.ScheduledAt)
	}
	if parentCall.Content != "main post {counter}" {
		t.Fatalf("parent content: want %q, got %q", "main post {counter}", parentCall.Content)
	}
	// Counter is passed to TemplateCounter for late expansion
	if parentCall.TemplateCounter == nil || *parentCall.TemplateCounter != 7 {
		t.Fatalf("parent TemplateCounter: want 7, got %v", parentCall.TemplateCounter)
	}
	// Second call: announcement at now - 2 days
	wantAnn := now.Add(-2 * 24 * time.Hour)
	annCall := st.createScheduledPostCalls[1]
	if !annCall.ScheduledAt.Equal(wantAnn) {
		t.Fatalf("announcement ScheduledAt: want %v, got %v", wantAnn, annCall.ScheduledAt)
	}
	// Announcement content is pre-expanded (including {main_*} from parent)
	// {main_month}=05 {main_day}=27 {main_weekday_name}=Wed
	wantContent := "episode #7 on 05/27 (Wed)"
	if annCall.Content != wantContent {
		t.Fatalf("announcement content: want %q, got %q", wantContent, annCall.Content)
	}
	if len(st.advancePostTemplateCalls) != 1 {
		t.Fatalf("expected 1 advancePostTemplate call, got %d", len(st.advancePostTemplateCalls))
	}
	if len(st.advanceAnnouncementCounterCalls) != 1 {
		t.Fatalf("expected 1 advanceAnnouncementCounter call, got %d", len(st.advanceAnnouncementCounterCalls))
	}
	if st.advanceAnnouncementCounterCalls[0].counterNext != 2 {
		t.Fatalf("announcement counter: want 2, got %d", st.advanceAnnouncementCounterCalls[0].counterNext)
	}
}

func strPtr(s string) *string { return &s }

func TestService_materializePostTemplates_skipWithoutShift(t *testing.T) {
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	tmpl := domain.PostTemplate{
		ID:                "tmpl2",
		TeamID:            "team1",
		AuthorUserID:      "user1",
		Title:             "skip-test",
		Content:           "skipped post",
		RecurrenceJSON:    `{"kind":"weekly","weekdays":[3],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:  []string{"acc1"},
		Enabled:           true,
		NextMaterializeAt: &now,
		CounterNext:       5,
	}

	st := &mockStore{
		listDuePostTemplates: []domain.PostTemplate{tmpl},
		isPostTemplateOccurrenceSkippedFn: func(templateID string, occurrenceAt time.Time) (bool, error) {
			return true, nil
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	err := svc.materializePostTemplates(context.Background())
	if err != nil {
		t.Fatalf("materializePostTemplates: %v", err)
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.createScheduledPostCalls) != 0 {
		t.Fatalf("expected 0 createScheduledPost calls (plain skip), got %d", len(st.createScheduledPostCalls))
	}
	if len(st.advancePostTemplateCalls) != 1 {
		t.Fatalf("expected 1 advancePostTemplate call, got %d", len(st.advancePostTemplateCalls))
	}
	if st.advancePostTemplateCalls[0].counterNext != 5 {
		t.Fatalf("expected counterNext=5 (unchanged), got %d", st.advancePostTemplateCalls[0].counterNext)
	}
}
