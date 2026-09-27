// Package push delivers standards-based Web Push (RFC 8030) notifications for
// new review items. Delivery failures never propagate to callers: review
// creation must not fail because a notification could not be sent. All
// failures surface in the structured log instead.
package push

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	defaultTTL             = 86400
	defaultSubject         = "mailto:goloom@localhost"
	defaultDeliveryTimeout = 10 * time.Second
	reviewURLScheme        = "/?team=%s&section=reviewQueue"
)

// pushStore is the minimal store surface the push sender depends on.
type pushStore interface {
	ListPushTargets(ctx context.Context, teamID string) ([]domain.PushSubscription, error)
	RetirePushSubscription(ctx context.Context, subID string) error
	CountOpenReviewItems(ctx context.Context, teamID string) (int, error)
	CountUserOpenReviewItems(ctx context.Context, userID string) (int, error)
	GetVAPIDKeys(ctx context.Context) (domain.VAPIDKeys, error)
	UpsertVAPIDKeys(ctx context.Context, keys domain.VAPIDKeys) error
}

// resolver resolves push endpoint hostnames. Defaulted to the system resolver;
// tests inject a fake one so private-address fixtures need no real DNS.
type resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Config holds the push sender settings. The optional PublicKey/PrivateKey
// pair pins explicit VAPID keys (env override); when both are empty a fresh
// pair is generated once and persisted so restart does not invalidate
// existing subscriptions.
type Config struct {
	Subject         string
	PublicKey       string
	PrivateKey      string
	HTTPClient      webpush.HTTPClient
	Resolver        resolver
	TTL             int
	DeliveryTimeout time.Duration
	Logger          *slog.Logger
}

// Sender delivers push notifications to eligible push subscriptions.
type Sender struct {
	store pushStore
	cfg   Config

	mu         sync.Mutex
	vapidInit  bool
	publicKey  string
	privateKey string
}

// topicForTeam derives the RFC 8030 Topic header for a team's notifications.
// The Topic value must use the URL-safe Base64 alphabet (RFC 4648 §5) and stay
// within 32 characters. A truncated SHA-256 over the team ID keeps the value
// stable per team (so bursts collapse into one push) and collision-resistant
// within the length budget.
func topicForTeam(teamID string) string {
	sum := sha256.Sum256([]byte(teamID))
	return base64.RawURLEncoding.EncodeToString(sum[:24])
}

// endpointLogLabel reduces a push endpoint to its origin only. The endpoint is
// a live per-device capability URL and never reaches the logs in full.
func endpointLogLabel(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "<invalid>"
	}
	return u.Scheme + "://" + u.Host
}

// Result summarizes one delivery run.
type Result struct {
	Delivered int
	Retired   int
	Failed    int
}

// ReviewNotification is the JSON payload sent for new review items. The body
// never contains post content — only team identity and the open count.
type ReviewNotification struct {
	Title string                  `json:"title"`
	Body  string                  `json:"body"`
	Data  ReviewNotificationData `json:"data"`
}

// ReviewNotificationData carries the routing metadata the service worker
// needs to group notifications and deep-link into the review queue. Count is
// this team's open review items (used for the unread notification body); Total
// is the authenticated user's sum across all their owner/editor teams, so a
// closed-app badge reflects the real user-wide count even when the app took a
// different path than the per-team count.
type ReviewNotificationData struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	Count    int    `json:"count"`
	Total    int    `json:"total"`
	PostID   string `json:"post_id"`
	URL      string `json:"url"`
}

// nonPublicPrefixes lists reserved, documentation, benchmarking and
// special-use ranges the stdlib NetAddr getters do not flag but an SSRF guard
// must still reject before a socket is opened. An endpoint may never resolve
// into one of these: a compromised client could otherwise point the server at
// an unreachable-but-local-ish target, making it an SSRF proxy. Sources: IANA
// special-purpose registries (RFC 5737 TEST-NET, RFC 6890 reserved,
// RFC 3849/RFC 9637 IPv6 documentation, RFC 2544/RFC 5180 benchmarking,
// RFC 6598 CGNAT, RFC 7526 6to4 relay anycast, RFC 3879 site-local,
// RFC 6666 discard-only). IPv4-mapped forms are normalized by Unmap before
// this table is consulted.
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network" on this host
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved for future use (incl. broadcast)
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT shared address space
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("192.88.99.0/24"),  // 6to4 relay anycast (deprecated)
	netip.MustParsePrefix("2001:db8::/32"),   // IPv6 documentation
	netip.MustParsePrefix("2001::/23"),       // reserved protocol assignments (incl. benchmarking)
	netip.MustParsePrefix("2001:ffff::/32"),  // reserved (deprecated overlay anycast)
	netip.MustParsePrefix("2002::/16"),       // 6to4 (deprecated)
	netip.MustParsePrefix("0100::/64"),       // discard-only
	netip.MustParsePrefix("3fff::/20"),       // reserved for future documentation
	netip.MustParsePrefix("fec0::/10"),       // site-local (deprecated)
}

// isPublicAddr reports whether addr is a routable public address. Loopback,
// RFC1918/ULA private ranges, link-local, multicast and unspecified addresses
// are non-public, as are the reserved/special-use ranges in nonPublicPrefixes:
// a push endpoint must never resolve to any of them, or the server becomes an
// SSRF proxy into an internal network (endpoints are stored capability URLs
// learned from browsers, so a compromised client could point the server at an
// internal host otherwise).
func isPublicAddr(addr netip.Addr) bool {
	// Unmap first so IPv4-in-IPv6 tuples are judged by their IPv4 value; a
	// mapped 8.8.8.8 is public, a mapped 10.0.0.1 is still SSRF fuel.
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsLoopback() ||
		addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return false
	}
	for _, p := range nonPublicPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return addr.IsGlobalUnicast()
}

// resolveHostIPs returns the addresses for an endpoint host. Hosts that are
// already IP literals are accepted directly; DNS failures surface as errors so
// a host that breaks mid-request can never fall through to a raw socket.
func resolveHostIPs(ctx context.Context, r resolver, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}, nil
	}
	addrs, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("no addresses resolved")
	}
	return addrs, nil
}

// guardedDialContext wraps the transport dialer so hosts resolve and dial only
// public addresses: any non-public resolved address refuses the connection
// before a socket is opened.
func guardedDialContext(base func(ctx context.Context, network, addr string) (net.Conn, error), r resolver) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("push: invalid endpoint address %q: %w", addr, err)
		}
		addrs, err := resolveHostIPs(ctx, r, host)
		if err != nil {
			return nil, fmt.Errorf("push: resolve endpoint %q: %w", host, err)
		}
		for _, a := range addrs {
			if !isPublicAddr(a) {
				return nil, fmt.Errorf("push: refusing non-public endpoint %q (%s)", host, a)
			}
		}
		return base(ctx, network, net.JoinHostPort(addrs[0].String(), port))
	}
}

// redirectGuard enforces the same SSRF policy on redirect targets: only https
// endpoints resolving to public addresses are followed.
func redirectGuard(r resolver) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("push: redirect to non-https endpoint")
		}
		addrs, err := resolveHostIPs(req.Context(), r, req.URL.Hostname())
		if err != nil {
			return fmt.Errorf("push: resolve redirect target %q: %w", req.URL.Hostname(), err)
		}
		for _, a := range addrs {
			if !isPublicAddr(a) {
				return fmt.Errorf("push: redirect to non-public endpoint %q", req.URL.Hostname())
			}
		}
		return nil
	}
}

// New creates a Sender backed by the given store subset.
func New(store pushStore, cfg Config) *Sender {
	if cfg.TTL <= 0 {
		cfg.TTL = defaultTTL
	}
	if cfg.DeliveryTimeout <= 0 {
		cfg.DeliveryTimeout = defaultDeliveryTimeout
	}
	if cfg.HTTPClient == nil {
		// Production transport: resolve and dial only public endpoint
		// addresses, reject redirects into non-public targets. Test suites
		// inject their own HTTPClient and keep the relay pattern.
		r := cfg.Resolver
		if r == nil {
			r = net.DefaultResolver
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = guardedDialContext(transport.DialContext, r)
		cfg.HTTPClient = &http.Client{
			Transport:     transport,
			Timeout:       cfg.DeliveryTimeout,
			CheckRedirect: redirectGuard(r),
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Subject == "" {
		cfg.Subject = defaultSubject
	}
	return &Sender{store: store, cfg: cfg}
}

// VAPIDPublicKey returns the public key clients subscribe with, loading or
// generating the key pair on first use.
func (s *Sender) VAPIDPublicKey(ctx context.Context) (string, error) {
	if err := s.ensureKeys(ctx); err != nil {
		return "", err
	}
	return s.publicKey, nil
}

func (s *Sender) ensureKeys(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vapidInit {
		return nil
	}

	// The initialized flag is only set after a successful load/generate: a
	// transient store failure must leave the sender un-initialized so a later
	// call can retry instead of pushing with empty keys.
	if s.cfg.PublicKey != "" && s.cfg.PrivateKey != "" {
		s.publicKey = s.cfg.PublicKey
		s.privateKey = s.cfg.PrivateKey
		s.vapidInit = true
		return nil
	}

	keys, err := s.store.GetVAPIDKeys(ctx)
	if err == nil {
		s.publicKey = keys.PublicKey
		s.privateKey = keys.PrivateKey
		s.vapidInit = true
		return nil
	}
	if !errors.Is(err, domain.ErrVAPIDKeysNotSet) {
		return fmt.Errorf("push: load VAPID keys: %w", err)
	}

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return fmt.Errorf("push: generate VAPID keys: %w", err)
	}
	if err := s.store.UpsertVAPIDKeys(ctx, domain.VAPIDKeys{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}); err != nil {
		return fmt.Errorf("push: persist VAPID keys: %w", err)
	}
	s.publicKey = publicKey
	s.privateKey = privateKey
	s.vapidInit = true
	s.cfg.Logger.Info("push: generated and persisted VAPID keys")
	return nil
}

// reviewPayload marshals the JSON body for a review notification. count is the
// per-team open count (also the unread notification body), total is the
// authenticated user's sum across all eligible teams (the app badge value).
func reviewPayload(team domain.Team, post domain.ScheduledPost, count, total int) ([]byte, error) {
	notif := ReviewNotification{
		Title: team.Name,
		Body:  fmt.Sprintf("%s · %d open", team.Name, count),
		Data: ReviewNotificationData{
			TeamID:   team.ID,
			TeamName: team.Name,
			Count:    count,
			Total:    total,
			PostID:   post.ID,
			URL:      fmt.Sprintf(reviewURLScheme, team.ID),
		},
	}
	if post.Title != "" {
		notif.Title = post.Title
	}
	return json.Marshal(notif)
}

// SendNewReview notifies all eligible subscriptions of the team about a new
// review item. The active notification is grouped per team (Topic header) so
// bursts collapse into one entry showing the current open count. Each target
// receives its authenticated user's badge total across all eligible teams.
// Delivery is bounded by one DeliveryTimeout budget for the whole batch, not
// per target, so stalled push services cannot multiply the timeout across
// subscriptions. Failures are logged, expired subscriptions are retired, and
// no error is returned so review creation itself is never blocked.
func (s *Sender) SendNewReview(ctx context.Context, team domain.Team, post domain.ScheduledPost) Result {
	var res Result
	count, err := s.store.CountOpenReviewItems(ctx, team.ID)
	if err != nil {
		s.cfg.Logger.Error("push: count open review items", "team_id", team.ID, "error", err)
		count = 1
	}
	targets, err := s.store.ListPushTargets(ctx, team.ID)
	if err != nil {
		s.cfg.Logger.Error("push: list push targets", "team_id", team.ID, "error", err)
		return res
	}
	if len(targets) == 0 {
		return res
	}
	if err := s.ensureKeys(ctx); err != nil {
		s.cfg.Logger.Error("push: send new review", "team_id", team.ID, "error", err)
		res.Failed = len(targets)
		return res
	}

	batchCtx, cancel := context.WithTimeout(ctx, s.cfg.DeliveryTimeout)
	defer cancel()

	topic := topicForTeam(team.ID)
	for _, target := range targets {
		total, err := s.store.CountUserOpenReviewItems(batchCtx, target.UserID)
		if err != nil {
			s.cfg.Logger.Error("push: count user open review items", "user_id", target.UserID, "error", err)
			total = count
		}
		payload, err := reviewPayload(team, post, count, total)
		if err != nil {
			s.cfg.Logger.Error("push: marshal notification", "team_id", team.ID, "error", err)
			res.Failed++
			continue
		}
		switch s.deliver(batchCtx, target, payload, topic) {
		case outcomeDelivered:
			res.Delivered++
		case outcomeRetired:
			res.Retired++
		default:
			res.Failed++
		}
	}
	s.cfg.Logger.Info("push: new review notification sent",
		"team_id", team.ID, "delivered", res.Delivered, "retired", res.Retired, "failed", res.Failed)
	return res
}

// SendTest sends the user a single test notification for the given device
// subscription. Ownership is verified so users can only test their own
// devices.
func (s *Sender) SendTest(ctx context.Context, userID string, sub domain.PushSubscription) error {
	if err := s.ensureKeys(ctx); err != nil {
		return err
	}
	notif := ReviewNotification{
		Title: "Test notification",
		Body:  "Push notifications are active on this device",
		Data:  ReviewNotificationData{URL: "/"},
	}
	payload, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("push: marshal test notification: %w", err)
	}
	if res := s.deliver(ctx, sub, payload, "review-test"); res != outcomeDelivered {
		return fmt.Errorf("push: test notification not delivered (%s)", res)
	}
	return nil
}

type outcome int

const (
	outcomeDelivered outcome = iota
	outcomeRetired
	outcomeFailed
)

func (o outcome) String() string {
	switch o {
	case outcomeDelivered:
		return "delivered"
	case outcomeRetired:
		return "retired"
	default:
		return "failed"
	}
}

func (s *Sender) deliver(ctx context.Context, sub domain.PushSubscription, payload []byte, topic string) outcome {
	// A bounded budget keeps a slow/unresponsive push service from hanging the
	// review-creation path; the context is the authority even for injected
	// HTTP clients that bring their own timeout.
	deliveryCtx, cancel := context.WithTimeout(ctx, s.cfg.DeliveryTimeout)
	defer cancel()
	resp, err := webpush.SendNotificationWithContext(deliveryCtx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, &webpush.Options{
		HTTPClient:      s.cfg.HTTPClient,
		Subscriber:      s.cfg.Subject,
		Topic:           topic,
		TTL:             s.cfg.TTL,
		Urgency:         webpush.UrgencyLow,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
	})
	if err != nil {
		s.cfg.Logger.Warn("push: send failed", "subscription_id", sub.ID, "endpoint", endpointLogLabel(sub.Endpoint), "error", err)
		return outcomeFailed
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		s.cfg.Logger.Debug("push: delivered", "subscription_id", sub.ID, "status", resp.StatusCode)
		return outcomeDelivered
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		s.cfg.Logger.Info("push: subscription retired (410/404)", "subscription_id", sub.ID, "endpoint", endpointLogLabel(sub.Endpoint), "status", resp.StatusCode)
		if err := s.store.RetirePushSubscription(ctx, sub.ID); err != nil {
			s.cfg.Logger.Warn("push: retire subscription", "subscription_id", sub.ID, "error", err)
		}
		return outcomeRetired
	default:
		// 400 bad payload, 401/403 auth, 429 rate limit, 5xx push service: keep
		// the subscription, surface the failure.
		s.cfg.Logger.Warn("push: endpoint rejected",
			"subscription_id", sub.ID, "endpoint", endpointLogLabel(sub.Endpoint), "status", resp.StatusCode)
		return outcomeFailed
	}
}