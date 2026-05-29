package aijobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

var defaultHTTPTransportBackoffs = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

type HTTPTransport struct {
	Client         *http.Client
	AttemptTimeout time.Duration
	RetryBackoffs  []time.Duration
	sleep          func(context.Context, time.Duration) error
}

type transportEnvelope struct {
	CallbackURL string          `json:"callback_url,omitempty"`
	Params      json.RawMessage `json:"params,omitempty"`
	Context     json.RawMessage `json:"context,omitempty"`
}

type httpDispatchPayload struct {
	JobID        string           `json:"job_id"`
	Type         domain.AIJobType `json:"type"`
	TeamID       string           `json:"team_id"`
	AuthorUserID string           `json:"author_user_id"`
	CallbackURL  string           `json:"callback_url"`
	Params       json.RawMessage  `json:"params"`
	Context      json.RawMessage  `json:"context,omitempty"`
}

func (t *HTTPTransport) Dispatch(ctx context.Context, job domain.AIJob, serviceURL string) error {
	payload, err := buildHTTPDispatchPayload(job)
	if err != nil {
		return fmt.Errorf("HTTPTransport.Dispatch: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("HTTPTransport.Dispatch: %w", err)
	}

	serviceURL = strings.TrimSpace(serviceURL)
	if serviceURL == "" {
		return fmt.Errorf("HTTPTransport.Dispatch: %w", fmt.Errorf("service URL is empty"))
	}

	backoffs := t.retryBackoffs()
	for attempt := 0; attempt < len(backoffs)+1; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, t.attemptTimeout())
		err = t.dispatchOnce(attemptCtx, serviceURL, body)
		cancel()
		if err == nil {
			return nil
		}

		if attempt == len(backoffs) {
			break
		}
		if sleepErr := t.sleepFn()(ctx, backoffs[attempt]); sleepErr != nil {
			return fmt.Errorf("HTTPTransport.Dispatch: %w", sleepErr)
		}
	}

	return fmt.Errorf("HTTPTransport.Dispatch: %w", err)
}

func (t *HTTPTransport) dispatchOnce(ctx context.Context, serviceURL string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	return nil
}

func buildHTTPDispatchPayload(job domain.AIJob) (httpDispatchPayload, error) {
	envelope, err := decodeTransportEnvelope(job.Payload)
	if err != nil {
		return httpDispatchPayload{}, err
	}
	return httpDispatchPayload{
		JobID:        job.ID,
		Type:         job.Type,
		TeamID:       job.TeamID,
		AuthorUserID: job.AuthorUserID,
		CallbackURL:  envelope.CallbackURL,
		Params:       ensureJSONObject(envelope.Params),
		Context:      envelope.Context,
	}, nil
}

func decodeTransportEnvelope(payload json.RawMessage) (transportEnvelope, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return transportEnvelope{Params: json.RawMessage(`{}`)}, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return transportEnvelope{}, err
	}

	if _, ok := fields["params"]; !ok {
		return transportEnvelope{Params: payload}, nil
	}

	envelope := transportEnvelope{
		Params:  ensureJSONObject(fields["params"]),
		Context: fields["context"],
	}
	if raw, ok := fields["callback_url"]; ok && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &envelope.CallbackURL); err != nil {
			return transportEnvelope{}, err
		}
	}
	return envelope, nil
}

func ensureJSONObject(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func (t *HTTPTransport) httpClient() *http.Client {
	if t.Client != nil {
		return t.Client
	}
	return &http.Client{}
}

func (t *HTTPTransport) attemptTimeout() time.Duration {
	if t.AttemptTimeout > 0 {
		return t.AttemptTimeout
	}
	return 30 * time.Second
}

func (t *HTTPTransport) retryBackoffs() []time.Duration {
	if len(t.RetryBackoffs) > 0 {
		return t.RetryBackoffs
	}
	return append([]time.Duration(nil), defaultHTTPTransportBackoffs...)
}

func (t *HTTPTransport) sleepFn() func(context.Context, time.Duration) error {
	if t.sleep != nil {
		return t.sleep
	}
	return sleepWithContext
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
