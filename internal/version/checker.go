package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultRepo is the GitHub repository the update check queries for releases.
const DefaultRepo = "Goloom-App/goloom"

// releaseAPITemplate is the GitHub REST endpoint for the newest published,
// non-prerelease release of a repository.
const releaseAPITemplate = "https://api.github.com/repos/%s/releases/latest"

// ReleaseStatus is the resolved update state exposed to clients: the running
// version, the newest known upstream release (empty until the first successful
// check), and whether an upgrade is available.
type ReleaseStatus struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
}

// ReleaseChecker periodically fetches the latest GitHub release and caches it,
// so the frontend can surface an update hint without every browser reaching out
// to GitHub (avoids per-client rate limits and phone-home). It is safe for
// concurrent use.
type ReleaseChecker struct {
	current  string
	apiURL   string
	client   *http.Client
	interval time.Duration
	log      *slog.Logger

	mu        sync.RWMutex
	latest    string
	checkedAt time.Time
}

// NewReleaseChecker builds a checker for the given repository ("owner/name").
func NewReleaseChecker(current, repo string, interval time.Duration, log *slog.Logger) *ReleaseChecker {
	return newReleaseChecker(
		current,
		fmt.Sprintf(releaseAPITemplate, repo),
		interval,
		&http.Client{Timeout: 10 * time.Second},
		log,
	)
}

func newReleaseChecker(current, apiURL string, interval time.Duration, client *http.Client, log *slog.Logger) *ReleaseChecker {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &ReleaseChecker{
		current:  current,
		apiURL:   apiURL,
		client:   client,
		interval: interval,
		log:      log,
	}
}

// Run performs an immediate check and then re-checks on the configured
// interval until ctx is cancelled. Intended to be launched in its own goroutine.
func (c *ReleaseChecker) Run(ctx context.Context) {
	c.refresh(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refresh(ctx)
		}
	}
}

// Status returns the current cached update state.
func (c *ReleaseChecker) Status() ReleaseStatus {
	c.mu.RLock()
	latest := c.latest
	c.mu.RUnlock()
	return ReleaseStatus{
		Current:         c.current,
		Latest:          latest,
		UpdateAvailable: updateAvailable(c.current, latest),
	}
}

// refresh fetches the latest release tag and updates the cache. Failures are
// logged and leave the previous cached value untouched — a transient GitHub
// outage must never turn into a spurious "up to date" or crash.
func (c *ReleaseChecker) refresh(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL, nil)
	if err != nil {
		c.log.Warn("release check: build request failed", "error", err)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "goloom-update-check")

	resp, err := c.client.Do(req)
	if err != nil {
		c.log.Warn("release check: request failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Warn("release check: unexpected status", "status", resp.StatusCode)
		return
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.log.Warn("release check: read body failed", "error", err)
		return
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		c.log.Warn("release check: decode failed", "error", err)
		return
	}

	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return
	}

	c.mu.Lock()
	c.latest = tag
	c.checkedAt = time.Now()
	c.mu.Unlock()
	c.log.Debug("release check: latest resolved", "latest", tag, "current", c.current)
}

// updateAvailable reports whether latest is a strictly newer release than
// current. Non-release builds (dev, dev-<rev>) and unparseable versions never
// trigger a hint, so local and CI builds are not nagged. Goloom tags are plain
// vMAJOR.MINOR.PATCH, so a three-field numeric compare is enough — no semver
// dependency required.
func updateAvailable(current, latest string) bool {
	if latest == "" {
		return false
	}
	cur, okCur := parseVersion(current)
	lat, okLat := parseVersion(latest)
	if !okCur || !okLat {
		return false
	}
	for i := 0; i < 3; i++ {
		if lat[i] != cur[i] {
			return lat[i] > cur[i]
		}
	}
	return false
}

// parseVersion parses a vMAJOR.MINOR.PATCH tag into its three numeric parts. A
// leading "v" is optional; anything else (pre-release suffixes, "dev", empty)
// is rejected so it cannot produce a misleading comparison.
func parseVersion(v string) ([3]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
