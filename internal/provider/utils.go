package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var defaultHTTPClient = &http.Client{Timeout: 15 * time.Second}

// RateLimitError indicates the provider returned HTTP 429 Too Many Requests.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited, retry after %s", e.RetryAfter)
	}
	return "rate limited (429)"
}

// IsRateLimitError returns true when err is a RateLimitError.
func IsRateLimitError(err error) bool {
	var rle *RateLimitError
	return errors.As(err, &rle)
}

// RetryPolicy controls rate-limit retry behaviour for provider API calls.
type RetryPolicy struct {
	MaxRetries int
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

// DefaultRetryPolicy provides sensible defaults for provider API rate-limit retries.
var DefaultRetryPolicy = RetryPolicy{
	MaxRetries: 3,
	MinBackoff: 1 * time.Second,
	MaxBackoff: 30 * time.Second,
}

// doRequestWithRetry calls fn with exponential backoff on 429 responses.
// After exhausting retries the last 429 response is returned without wrapping
// so the caller can still inspect the body.
func doRequestWithRetry(ctx context.Context, policy RetryPolicy, fn func(context.Context) (*http.Response, error)) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := policy.MinBackoff

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err = fn(ctx)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		if attempt == policy.MaxRetries {
			return resp, nil
		}

		// Drain and close body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		// Respect Retry-After header if present.
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, parseErr := strconv.Atoi(ra); parseErr == nil {
				backoff = time.Duration(secs) * time.Second
				if backoff > policy.MaxBackoff {
					backoff = policy.MaxBackoff
				}
				continue
			}
		}
		backoff *= 2
		if backoff > policy.MaxBackoff {
			backoff = policy.MaxBackoff
		}
	}
	return resp, nil
}

func normalizeInstanceURL(ctx context.Context, raw string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("instance_url is required")
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse instance_url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("instance_url must include a valid host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if err := validateInstanceOutboundURL(ctx, parsed); err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func marshalJSONBody(payload any) (*bytes.Reader, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}
	return bytes.NewReader(body), nil
}

func doJSONRequest(ctx context.Context, method, endpoint, bearerToken string, body *bytes.Reader) (*http.Response, error) {
	return doRequestWithRetry(ctx, DefaultRetryPolicy, func(ctx context.Context) (*http.Response, error) {
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			reader = body
		}

		req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := defaultHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		return resp, nil
	})
}

func cleanScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		normalized := strings.TrimSpace(scope)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func providerInstanceID(instance *domain.ProviderInstance) string {
	if instance == nil {
		return ""
	}
	return instance.ID
}
