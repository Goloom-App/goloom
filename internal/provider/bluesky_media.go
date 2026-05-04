package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	blueskyMediaIDPrefix  = "goloom-bsky:"
	blueskyMaxUploadBytes = 8 << 20
)

type blueskyUploadedMediaPayload struct {
	Blob map[string]any `json:"blob"`
	Alt  string         `json:"alt,omitempty"`
}

func encodeBlueskyMediaID(blob map[string]any, alt string) (string, error) {
	if blob == nil {
		return "", errors.New("missing blob")
	}
	p := blueskyUploadedMediaPayload{Blob: blob, Alt: alt}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return blueskyMediaIDPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeBlueskyMediaIDs(ids []string) ([]blueskyUploadedMediaPayload, error) {
	var out []blueskyUploadedMediaPayload
	for _, raw := range ids {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.HasPrefix(raw, blueskyMediaIDPrefix) {
			return nil, fmt.Errorf("invalid bluesky media id (expected prefix %s)", blueskyMediaIDPrefix)
		}
		enc := strings.TrimPrefix(raw, blueskyMediaIDPrefix)
		b, err := base64.RawURLEncoding.DecodeString(enc)
		if err != nil {
			return nil, fmt.Errorf("decode bluesky media id: %w", err)
		}
		var p blueskyUploadedMediaPayload
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("parse bluesky media payload: %w", err)
		}
		if p.Blob == nil {
			return nil, errors.New("bluesky media payload missing blob")
		}
		out = append(out, p)
	}
	return out, nil
}

func blueskyUploadBlob(ctx context.Context, instanceURL, accessJWT string, data []byte, contentType string) (map[string]any, error) {
	endpoint := strings.TrimRight(instanceURL, "/") + "/xrpc/com.atproto.repo.uploadBlob"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessJWT))
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	req.Header.Set("Content-Type", ct)
	req.ContentLength = int64(len(data))

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("uploadBlob failed status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	var out struct {
		Blob map[string]any `json:"blob"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode uploadBlob: %w", err)
	}
	if out.Blob == nil {
		return nil, errors.New("uploadBlob returned empty blob")
	}
	return out.Blob, nil
}
