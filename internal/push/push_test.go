package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	webpush "github.com/SherClockHolmes/webpush-go"
)

type fakeStore struct {
	mu             sync.Mutex
	targets        []domain.PushSubscription
	count          int
	countErr       error
	countPerTeam   map[string]int
	userTotals     map[string]int
	userTotalCalls []string
	vapidKeys      domain.VAPIDKeys
	vapidErr       error
	vapidErrCalls  int
	upserted       domain.VAPIDKeys
	upsertedSet    bool
	upsertErr      error
	retired        []string
}

func (f *fakeStore) ListPushTargets(_ context.Context, _ string) ([]domain.PushSubscription, error) {
	return f.targets, nil
}

func (f *fakeStore) RetirePushSubscription(_ context.Context, subID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retired = append(f.retired, subID)
	return nil
}

func (f *fakeStore) CountOpenReviewItems(_ context.Context, teamID string) (int, error) {
	if f.countPerTeam != nil {
		return f.countPerTeam[teamID], f.countErr
	}
	return f.count, f.countErr
}

func (f *fakeStore) CountUserOpenReviewItems(_ context.Context, userID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userTotalCalls = append(f.userTotalCalls, userID)
	if f.userTotals != nil {
		return f.userTotals[userID], nil
	}
	return 0, nil
}

func (f *fakeStore) GetVAPIDKeys(_ context.Context) (domain.VAPIDKeys, error) {
	if f.vapidErrCalls > 0 {
		f.vapidErrCalls--
		return domain.VAPIDKeys{}, errors.New("transient store failure")
	}
	if f.vapidErr != nil {
		return f.vapidKeys, f.vapidErr
	}
	if f.vapidKeys.PublicKey == "" {
		return domain.VAPIDKeys{}, domain.ErrVAPIDKeysNotSet
	}
	return f.vapidKeys, nil
}

func (f *fakeStore) UpsertVAPIDKeys(_ context.Context, keys domain.VAPIDKeys) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted = keys
	f.upsertedSet = true
	return nil
}

func validKeys(t *testing.T) (auth, p256dh string) {
	t.Helper()
	secret := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		t.Fatal(err)
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	point := elliptic.Marshal(elliptic.P256(), priv.X, priv.Y)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(secret), enc.EncodeToString(point)
}

func subFor(t *testing.T, endpoint string) domain.PushSubscription {
	t.Helper()
	auth, p256dh := validKeys(t)
	return domain.PushSubscription{
		ID:       subID(endpoint),
		Endpoint: endpoint,
		Auth:     auth,
		P256dh:   p256dh,
		Enabled:  true,
	}
}

// subID derives a short deterministic subscription ID so test fixtures never
// embed the (capability) endpoint in the ID they assert on.
func subID(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return "sub-" + base64.RawURLEncoding.EncodeToString(sum[:6])
}

func newServer(t *testing.T, status int, seen func(*http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			seen(r)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func quietLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// relayFor returns an HTTP client that forwards every delivery request to the
// given httptest server. The production default client is SSRF-guarded (see
// guardedDialContext), so tests exercising delivery routes via a loopback
// endpoint must inject a client — which the Config contract allows.
func relayFor(srv *httptest.Server) webpush.HTTPClient {
	return &relayClient{target: srv}
}

type relayClient struct {
	target *httptest.Server
}

func (c *relayClient) Do(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = "http"
	u.Host = c.target.Listener.Addr().String()
	clone.URL = &u
	return c.target.Client().Transport.RoundTrip(clone)
}

// subForUser returns a valid subscription bound to a user, for per-target
// assertions.
func subForUser(t *testing.T, userID, endpoint string) domain.PushSubscription {
	t.Helper()
	sub := subFor(t, endpoint)
	sub.UserID = userID
	return sub
}

type fakeResolver struct {
	addrs map[string][]netip.Addr
	err   error
}

func (f fakeResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.addrs[host], nil
}

func TestSendNewReview_DeliversToAllTargets(t *testing.T) {
	srv := newServer(t, http.StatusCreated, nil)
	store := &fakeStore{
		count: 3,
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/1"),
			subFor(t, srv.URL+"/2"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	team := domain.Team{ID: "team-1", Name: "My Team"}
	post := domain.ScheduledPost{ID: "post-1", Title: "Draft title"}

	res := s.SendNewReview(context.Background(), team, post)
	if res.Delivered != 2 || res.Retired != 0 || res.Failed != 0 {
		t.Fatalf("result: %#v", res)
	}
	if !store.upsertedSet {
		t.Fatal("VAPID keys should have been generated and persisted")
	}
	if store.upserted.PublicKey == "" || store.upserted.PrivateKey == "" {
		t.Fatalf("upserted keys: %#v", store.upserted)
	}
}

func TestSendNewReview_RequestBodyAndHeaders(t *testing.T) {
	var reqs []*http.Request
	var mu sync.Mutex
	srv := newServer(t, http.StatusCreated, func(r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r)
		mu.Unlock()
	})
	store := &fakeStore{
		count: 5,
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/1"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "My Team"}, domain.ScheduledPost{ID: "post-1", Title: "Draft title"})
	if res.Delivered != 1 {
		t.Fatalf("result: %#v", res)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reqs) != 1 {
		t.Fatalf("requests: %d", len(reqs))
	}
	r := reqs[0]
	if r.Header.Get("Authorization") == "" {
		t.Error("missing VAPID Authorization header")
	}
	if r.Header.Get("Topic") != topicForTeam("team-1") {
		t.Errorf("Topic header: %q", r.Header.Get("Topic"))
	}
	if r.Header.Get("TTL") != "86400" {
		t.Errorf("TTL header: %q", r.Header.Get("TTL"))
	}
	if r.Header.Get("Urgency") != "low" {
		t.Errorf("Urgency header: %q", r.Header.Get("Urgency"))
	}
}

func TestSendNewReview_PayloadShape(t *testing.T) {
	srv := newServer(t, http.StatusCreated, nil)
	store := &fakeStore{
		count: 5,
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/1"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "My Team"}, domain.ScheduledPost{ID: "post-1", Title: "Draft title"})
	if res.Delivered != 1 {
		t.Fatalf("result: %#v", res)
	}

	// The wire payload is aes128gcm-encrypted, so assert on the struct that
	// the scheduler marshals: team identity + open count, post title only.
	notif := ReviewNotification{
		Title: "Draft title",
		Body:  "My Team · 5 open",
		Data: ReviewNotificationData{
			TeamID:   "team-1",
			TeamName: "My Team",
			Count:    5,
			PostID:   "post-1",
			URL:      "/?team=team-1&section=reviewQueue",
		},
	}
	b, err := json.Marshal(notif)
	if err != nil {
		t.Fatal(err)
	}
	var round domainJSON
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	if round.Body != "My Team · 5 open" {
		t.Errorf("body: %q", round.Body)
	}
	if round.Title != "Draft title" {
		t.Errorf("title: %q", round.Title)
	}
	if round.Data.URL != "/?team=team-1&section=reviewQueue" {
		t.Errorf("url: %q", round.Data.URL)
	}
	if round.Data.TeamID != "team-1" || round.Data.PostID != "post-1" {
		t.Errorf("data: %#v", round.Data)
	}
}

type domainJSON struct {
	Title string                 `json:"title"`
	Body  string                 `json:"body"`
	Data  ReviewNotificationData `json:"data"`
}

func TestSendNewReview_RetiresGoneSubscriptions(t *testing.T) {
	srv := newServer(t, http.StatusGone, nil)
	store := &fakeStore{
		count: 1,
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/gone"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	if res.Retired != 1 || res.Delivered != 0 {
		t.Fatalf("result: %#v", res)
	}
	if len(store.retired) != 1 || store.retired[0] != subID(srv.URL+"/gone") {
		t.Fatalf("retired: %#v", store.retired)
	}
}

func TestSendNewReview_NotFoundRetiresToo(t *testing.T) {
	srv := newServer(t, http.StatusNotFound, nil)
	store := &fakeStore{
		count: 1,
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/gone"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	if res.Retired != 1 {
		t.Fatalf("result: %#v", res)
	}
}

func TestSendNewReview_RejectedEndpointKeepsSubscription(t *testing.T) {
	srv := newServer(t, http.StatusForbidden, nil)
	store := &fakeStore{
		count: 1,
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/denied"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	if res.Failed != 1 || res.Retired != 0 {
		t.Fatalf("result: %#v", res)
	}
	if len(store.retired) != 0 {
		t.Fatalf("must not retire rejected subscriptions: %#v", store.retired)
	}
}

func TestSendNewReview_CountFallbackOnError(t *testing.T) {
	srv := newServer(t, http.StatusOK, nil)
	store := &fakeStore{
		countErr: errors.New("boom"),
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/1"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	if res.Delivered != 1 {
		t.Fatalf("count error must not stop delivery: %#v", res)
	}
}

func TestSendNewReview_NoTargetsNoop(t *testing.T) {
	s := New(&fakeStore{count: 2}, Config{Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	if res != (Result{}) {
		t.Fatalf("empty targets must produce empty result: %#v", res)
	}
}

func TestSendNewReview_TopicHeaderPerTeam(t *testing.T) {
	var topics []string
	var mu sync.Mutex
	srv := newServer(t, http.StatusCreated, func(r *http.Request) {
		mu.Lock()
		topics = append(topics, r.Header.Get("Topic"))
		mu.Unlock()
	})
	store := &fakeStore{
		count:  1,
		targets: []domain.PushSubscription{subFor(t, srv.URL + "/1")},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	s.SendNewReview(context.Background(), domain.Team{ID: "team-A", Name: "A"}, domain.ScheduledPost{ID: "p"})
	mu.Lock()
	defer mu.Unlock()
	if len(topics) != 1 || topics[0] != topicForTeam("team-A") {
		t.Fatalf("topics: %#v", topics)
	}
}

func TestVAPIDPublicKey_PinsConfiguredKeys(t *testing.T) {
	store := &fakeStore{vapidErr: domain.ErrVAPIDKeysNotSet}
	s := New(store, Config{PublicKey: "pub-pinned", PrivateKey: "priv-pinned", Logger: quietLogger()})
	key, err := s.VAPIDPublicKey(context.Background())
	if err != nil || key != "pub-pinned" {
		t.Fatalf("pinned key: %q %v", key, err)
	}
	if store.upsertedSet {
		t.Fatal("pinned keys must not be persisted")
	}
}

func TestVAPIDPublicKey_GeneratesAndPersists(t *testing.T) {
	store := &fakeStore{vapidErr: domain.ErrVAPIDKeysNotSet}
	s := New(store, Config{Logger: quietLogger()})
	key, err := s.VAPIDPublicKey(context.Background())
	if err != nil || key == "" {
		t.Fatalf("generated key: %q %v", key, err)
	}
	if !store.upsertedSet || store.upserted.PublicKey != key {
		t.Fatalf("persisted: %#v", store.upserted)
	}

	// A fresh store with persisted keys must reuse them, not regenerate.
	store2 := &fakeStore{vapidKeys: store.upserted}
	s2 := New(store2, Config{Logger: quietLogger()})
	key2, err := s2.VAPIDPublicKey(context.Background())
	if err != nil || key2 != key {
		t.Fatalf("reused key: %q %v", key2, err)
	}
	if store2.upsertedSet {
		t.Fatal("must not regenerate on second load")
	}
}

func TestSendTest_Delivers(t *testing.T) {
	srv := newServer(t, http.StatusCreated, nil)
	store := &fakeStore{vapidErr: domain.ErrVAPIDKeysNotSet}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	err := s.SendTest(context.Background(), "user-1", subFor(t, srv.URL+"/1"))
	if err != nil {
		t.Fatalf("send test: %v", err)
	}
}

func TestSendTest_FailureIsReturned(t *testing.T) {
	srv := newServer(t, http.StatusBadRequest, nil)
	store := &fakeStore{vapidErr: domain.ErrVAPIDKeysNotSet}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	err := s.SendTest(context.Background(), "user-1", subFor(t, srv.URL+"/1"))
	if err == nil {
		t.Fatal("test send failure must be returned so the UI can show it")
	}
}

var urlSafeBase64 = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func TestTopicForTeam_RFC8030Constraints(t *testing.T) {
	topics := []string{
		topicForTeam("team-1"),
		topicForTeam("team-A"),
		topicForTeam("550e8400-e29b-41d4-a716-446655440000"),
	}
	for _, topic := range topics {
		if len(topic) > 32 {
			t.Errorf("topic %q exceeds RFC 8030 32-char limit (%d)", topic, len(topic))
		}
		if !urlSafeBase64.MatchString(topic) {
			t.Errorf("topic %q is not URL-safe Base64", topic)
		}
	}
	// Stable per team, so a burst collapses into one push.
	if topicForTeam("team-1") != topicForTeam("team-1") {
		t.Error("topic must be stable for the same team")
	}
	if topicForTeam("team-1") == topicForTeam("team-A") {
		t.Error("distinct teams must not collide")
	}
}

func TestVAPIDPublicKey_RetriesAfterTransientFailure(t *testing.T) {
	store := &fakeStore{vapidErr: domain.ErrVAPIDKeysNotSet, vapidErrCalls: 1}
	s := New(store, Config{Logger: quietLogger()})

	if _, err := s.VAPIDPublicKey(context.Background()); err == nil {
		t.Fatal("transient load failure must surface as an error")
	}
	// The failed attempt must not have marked the sender initialized: a retry
	// succeeds and generates keys instead of serving empty ones.
	key, err := s.VAPIDPublicKey(context.Background())
	if err != nil || key == "" {
		t.Fatalf("retry must succeed: %q %v", key, err)
	}
}

func TestVAPIDPublicKey_NotInitializedOnPersistFailure(t *testing.T) {
	store := &fakeStore{vapidErr: domain.ErrVAPIDKeysNotSet, upsertErr: errors.New("persist boom")}
	s := New(store, Config{Logger: quietLogger()})
	if _, err := s.VAPIDPublicKey(context.Background()); err == nil {
		t.Fatal("persist failure must surface as an error")
	}
	store.upsertErr = nil
	key, err := s.VAPIDPublicKey(context.Background())
	if err != nil || key == "" {
		t.Fatalf("retry after persist failure must succeed: %q %v", key, err)
	}
}

func TestSendNewReview_DeliveryTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	store := &fakeStore{
		count:  1,
		targets: []domain.PushSubscription{subFor(t, srv.URL+"/slow")},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger(), DeliveryTimeout: 50 * time.Millisecond})

	start := time.Now()
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	elapsed := time.Since(start)
	if res.Failed != 1 || res.Delivered != 0 {
		t.Fatalf("result: %#v", res)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("delivery was not bounded by timeout: %s", elapsed)
	}
}

func TestSendNewReview_LogsEndpointOriginOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	srv := newServer(t, http.StatusForbidden, nil)
	store := &fakeStore{
		count:  1,
		targets: []domain.PushSubscription{subFor(t, srv.URL+"/secret-capability-xyz")},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: logger})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	if res.Failed != 1 {
		t.Fatalf("result: %#v", res)
	}
	logOutput := buf.String()
	if strings.Contains(logOutput, "secret-capability-xyz") {
		t.Fatal("full capability endpoint leaked into logs")
	}
	if !strings.Contains(logOutput, endpointLogLabel(srv.URL)) {
		t.Fatalf("logs must keep the endpoint origin: %q", logOutput)
	}
}

func TestReviewPayload_EmbedsUserTotal(t *testing.T) {
	payload, err := reviewPayload(
		domain.Team{ID: "team-1", Name: "My Team"},
		domain.ScheduledPost{ID: "post-1", Title: "Draft title"},
		2, 7,
	)
	if err != nil {
		t.Fatal(err)
	}
	var round ReviewNotification
	if err := json.Unmarshal(payload, &round); err != nil {
		t.Fatal(err)
	}
	if round.Data.Count != 2 || round.Data.Total != 7 {
		t.Fatalf("data: %#v, want count 2 / total 7", round.Data)
	}
	if round.Body != "My Team · 2 open" {
		t.Errorf("body must stay per-team: %q", round.Body)
	}
}

func TestSendNewReview_QueriesUserTotalPerTarget(t *testing.T) {
	srv := newServer(t, http.StatusCreated, nil)
	store := &fakeStore{
		count: 3,
		userTotals: map[string]int{"user-a": 5, "user-b": 2},
		targets: []domain.PushSubscription{
			subForUser(t, "user-a", srv.URL+"/a"),
			subForUser(t, "user-b", srv.URL+"/b"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger()})
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	if res.Delivered != 2 {
		t.Fatalf("result: %#v", res)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.userTotalCalls) != 2 || store.userTotalCalls[0] != "user-a" || store.userTotalCalls[1] != "user-b" {
		t.Fatalf("user totals must be queried per target user: %#v", store.userTotalCalls)
	}
}

func TestSendNewReview_BatchTimeoutBoundsAllTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	store := &fakeStore{
		count: 1,
		targets: []domain.PushSubscription{
			subFor(t, srv.URL+"/1"),
			subFor(t, srv.URL+"/2"),
			subFor(t, srv.URL+"/3"),
		},
	}
	s := New(store, Config{HTTPClient: relayFor(srv), Logger: quietLogger(), DeliveryTimeout: 300 * time.Millisecond})

	start := time.Now()
	res := s.SendNewReview(context.Background(), domain.Team{ID: "team-1", Name: "T"}, domain.ScheduledPost{ID: "p"})
	elapsed := time.Since(start)
	if res.Failed != 3 || res.Delivered != 0 {
		t.Fatalf("result: %#v", res)
	}
	// Serial per-target timeouts would need ~3x the budget; one batch budget
	// must cut the whole stalled batch down to a single DeliveryTimeout.
	if elapsed > 700*time.Millisecond {
		t.Fatalf("batch was not bounded by a single timeout: %s", elapsed)
	}
}

func TestIsPublicAddr(t *testing.T) {
	cases := []struct {
		addr   string
		public bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"93.184.216.34", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"10.0.0.7", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"192.168.1.1", false},
		{"169.254.1.1", false},
		{"fe80::1", false},
		{"fc00::1", false},
		{"fd12:3456::1", false},
		{"0.0.0.0", false},
		{"0.128.0.1", false},
		{"224.0.0.1", false},
		{"ff02::1", false},
		{"255.255.255.255", false},
		{"240.0.0.1", false},
		{"241.7.8.9", false},
		// CGNAT shared address space (RFC 6598) — reachable for some carriers,
		// private by design.
		{"100.64.0.1", false},
		{"100.127.255.254", false},
		{"100.128.0.1", true},
		// Documentation / TEST-NET (RFC 5737) and benchmarking (RFC 2544).
		{"192.0.2.55", false},
		{"198.51.100.55", false},
		{"203.0.113.55", false},
		{"198.18.0.1", false},
		{"198.19.255.255", false},
		{"198.20.0.1", true},
		// Deprecated 6to4 relay anycast (RFC 7526).
		{"192.88.99.1", false},
		{"193.0.0.1", true},
		// IPv6 documentation (RFC 3849), benchmarking, 6to4, reserved protocol
		// assignment space, discard-only and future documentation blocks.
		{"2001:db8::1", false},
		{"2001:2::1", false},
		{"2002::1", false},
		{"2001:ffff::1", false},
		{"2001:0100::1", false},
		{"0100::1", false},
		{"3fff::1", false},
		{"2606:4700:4700::1112", true},
		{"2a00:1450:4001::1", true},
		// IPv4-mapped forms must be judged by the mapped IPv4 value: a mapped
		// public address is public, mapped private/link-local/CGNAT/doc are not.
		{"::ffff:8.8.8.8", true},
		{"::ffff:127.0.0.1", false},
		{"::ffff:10.0.0.1", false},
		{"::ffff:169.254.1.1", false},
		{"::ffff:100.64.0.1", false},
		{"::ffff:192.0.2.9", false},
		{"::ffff:224.0.0.1", false},
	}
	for _, tc := range cases {
		addr, err := netip.ParseAddr(tc.addr)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.addr, err)
		}
		if got := isPublicAddr(addr); got != tc.public {
			t.Errorf("isPublicAddr(%s) = %v, want %v", tc.addr, got, tc.public)
		}
	}
}

func TestGuardedDialContext_RejectsNonPublic(t *testing.T) {
	r := fakeResolver{addrs: map[string][]netip.Addr{
		"internal.corp": {netip.MustParseAddr("10.0.0.7")},
		"mixed":         {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.7")},
	}}
	dialed := false
	dial := guardedDialContext(func(_ context.Context, _, _ string) (net.Conn, error) {
		dialed = true
		return nil, nil
	}, r)
	for _, host := range []string{"internal.corp:443", "mixed:443", "127.0.0.1:443", "[::1]:443", "192.168.0.5:443"} {
		if _, err := dial(context.Background(), "tcp", host); err == nil {
			t.Errorf("host %s: expected refusal", host)
		}
	}
	if dialed {
		t.Error("base dialer must never run for non-public endpoints")
	}
}

func TestGuardedDialContext_DialsResolvedPublicHost(t *testing.T) {
	r := fakeResolver{addrs: map[string][]netip.Addr{
		"cdn.example": {netip.MustParseAddr("93.184.216.34")},
	}}
	var got string
	dial := guardedDialContext(func(_ context.Context, _, addr string) (net.Conn, error) {
		got = addr
		return nil, errors.New("stop")
	}, r)
	if _, err := dial(context.Background(), "tcp", "cdn.example:443"); err == nil {
		t.Fatal("expected the base dialer error")
	}
	if got != "93.184.216.34:443" {
		t.Fatalf("dialed %q, want the resolved public IP", got)
	}
}

func TestGuardedDialContext_UnresolvableHostFails(t *testing.T) {
	dial := guardedDialContext(func(_ context.Context, _, _ string) (net.Conn, error) { return nil, nil }, fakeResolver{err: errors.New("dns down")})
	if _, err := dial(context.Background(), "tcp", "nowhere.example:443"); err == nil {
		t.Fatal("DNS failure must fail the dial, never fall through to a raw socket")
	}
}

func TestRedirectGuard_RejectsNonPublicEndpoints(t *testing.T) {
	guard := redirectGuard(fakeResolver{addrs: map[string][]netip.Addr{
		"pub.example": {netip.MustParseAddr("93.184.216.34")},
	}})
	if err := guard(httptest.NewRequest("GET", "https://pub.example/x", nil), nil); err != nil {
		t.Errorf("public redirect rejected: %v", err)
	}
	if err := guard(httptest.NewRequest("GET", "https://10.0.0.1/x", nil), nil); err == nil {
		t.Error("redirect to a private IP must be refused")
	}
	if err := guard(httptest.NewRequest("GET", "http://pub.example/x", nil), nil); err == nil {
		t.Error("redirect to an http endpoint must be refused")
	}
}