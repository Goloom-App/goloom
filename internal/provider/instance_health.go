package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// InstanceHealthStatus is a coarse, UI-friendly classification of a reachability probe.
type InstanceHealthStatus string

const (
	InstanceHealthOK          InstanceHealthStatus = "ok"
	InstanceHealthUnreachable InstanceHealthStatus = "unreachable"
	InstanceHealthInvalidURL  InstanceHealthStatus = "invalid_url"
	InstanceHealthError       InstanceHealthStatus = "error"
)

// InstanceHealth is the result of an on-demand provider-instance reachability probe.
type InstanceHealth struct {
	Healthy bool                 `json:"healthy"`
	Status  InstanceHealthStatus `json:"status"`
	Detail  string               `json:"detail,omitempty"`
}

// instanceHealthPath returns a public, unauthenticated endpoint that signals the
// instance is up for the given provider.
func instanceHealthPath(providerName string) string {
	if strings.EqualFold(strings.TrimSpace(providerName), "bluesky") {
		// AT Protocol PDS describe endpoint.
		return "/xrpc/com.atproto.server.describeServer"
	}
	// Mastodon-compatible providers (mastodon, pixelfed, friendica).
	return "/api/v1/instance"
}

// CheckInstanceHealth probes a provider instance for basic reachability using a
// provider-appropriate public endpoint. It honours the outbound SSRF policy carried
// on ctx (see WithOutboundInstancePolicy) and never returns an error — the result is
// always a classified InstanceHealth so callers can render it directly.
func CheckInstanceHealth(ctx context.Context, providerName, instanceURL string) InstanceHealth {
	normalized, err := normalizeInstanceURL(ctx, instanceURL)
	if err != nil {
		return InstanceHealth{Healthy: false, Status: InstanceHealthInvalidURL, Detail: err.Error()}
	}

	endpoint := normalized + instanceHealthPath(providerName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return InstanceHealth{Healthy: false, Status: InstanceHealthError, Detail: err.Error()}
	}
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return InstanceHealth{Healthy: false, Status: InstanceHealthUnreachable, Detail: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return InstanceHealth{Healthy: true, Status: InstanceHealthOK}
	}
	return InstanceHealth{Healthy: false, Status: InstanceHealthError, Detail: fmt.Sprintf("status %d", resp.StatusCode)}
}
