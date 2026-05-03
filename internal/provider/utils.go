package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var defaultHTTPClient = &http.Client{Timeout: 15 * time.Second}

func normalizeInstanceURL(raw string) (string, error) {
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
