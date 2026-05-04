package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
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

func (m *mockStore) ListTeamMembers(ctx context.Context, teamID string) ([]domain.TeamMembership, error) {
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

func (m *mockStore) GetAccountsByIDs(ctx context.Context, teamID string, ids []string) ([]domain.SocialAccount, error) {
	return nil, nil
}

func (m *mockStore) CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	return domain.ScheduledPost{}, nil
}

func (m *mockStore) ListTeamPosts(ctx context.Context, teamID string) ([]domain.ScheduledPost, error) {
	return nil, nil
}

func (m *mockStore) GetScheduledPost(ctx context.Context, teamID, postID string) (domain.ScheduledPost, error) {
	return domain.ScheduledPost{}, nil
}

func (m *mockStore) UpdateScheduledPost(ctx context.Context, teamID, postID string, input domain.CreatePostInput) (domain.ScheduledPost, error) {
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

func (m *mockStore) MarkPostTargetResult(ctx context.Context, postID, accountID string, status domain.PostStatus, publishedURL, lastError string, publishMetadata map[string]string) error {
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

func (m *mockStore) UpsertPostMetrics(ctx context.Context, postID, accountID string, metrics map[string]int64) error {
	return nil
}

func (m *mockStore) MarkScheduledPostTargetMetricsSynced(ctx context.Context, postID, accountID, utcDay string) error {
	return nil
}

func (m *mockStore) ListOAuthAccountsWithAccessTokenExpiringBefore(ctx context.Context, before time.Time, limit int) ([]domain.AccountOAuthTokenExpiry, error) {
	return nil, nil
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

func (m *mockStore) CreateUserAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time) (string, domain.APIToken, error) {
	return "", domain.APIToken{}, nil
}

func (m *mockStore) ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	return nil, nil
}

func (m *mockStore) RevokeUserAPIToken(ctx context.Context, userID, tokenID string) error {
	return nil
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

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew_workersDefault(t *testing.T) {
	t.Parallel()
	st := &mockStore{}
	reg := provider.NewRegistry()
	svc := New(testLogger(), st, reg, time.Minute, 0, 0, 0)
	if svc.workers != 1 {
		t.Fatalf("workers want 1, got %d", svc.workers)
	}
	svcNeg := New(testLogger(), st, reg, time.Minute, -3, 0, 0)
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
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 2, 0, 0)
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
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0)
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
	svc := New(testLogger(), st, provider.NewRegistry(), 20*time.Millisecond, 1, 0, 0)
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
