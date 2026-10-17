package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

type auditRequestKey struct{}
type auditRequestState struct {
	err       error
	requestID string
	fields    string
}

type auditResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *auditResponse) Header() http.Header { return w.header }
func (w *auditResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *auditResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

var errAuditRejected = errors.New("audited mutation rejected")

// auditedHandler buffers the response until both business state and evidence commit.
// Only handlers without outbound provider calls belong here. Provider handlers use
// it around their database-only tail, after the external result is known.
func (a *API) auditedHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := &auditRequestState{requestID: uuid.NewString()}
		// Capture field names only. Bodies, URLs, prompts and values never enter events.
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") && r.Body != nil {
			raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
			if err != nil {
				a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), r.Body))
			var input map[string]json.RawMessage
			if json.Unmarshal(raw, &input) == nil {
				fields := make([]string, 0, len(input))
				for key := range input {
					if auditFieldNames[key] {
						fields = append(fields, key)
					}
				}
				sort.Strings(fields)
				state.fields = strings.Join(fields, ",")
			}
		}
		response := &auditResponse{header: make(http.Header)}
		err := a.store.WithAuditTransaction(r.Context(), func(ctx context.Context) error {
			ctx = context.WithValue(ctx, auditRequestKey{}, state)
			next(response, r.WithContext(ctx))
			if state.err != nil {
				return state.err
			}
			if response.status >= 400 {
				return errAuditRejected
			}
			return nil
		})
		if err != nil && !errors.Is(err, errAuditRejected) {
			a.log.ErrorContext(r.Context(), "audited mutation rolled back", "request_id", state.requestID, "error", err)
			a.writeError(w, r, "internal_error", http.StatusInternalServerError)
			return
		}
		for key, values := range response.header {
			w.Header()[key] = values
		}
		if response.status != 0 {
			w.WriteHeader(response.status)
		}
		_, _ = w.Write(response.body.Bytes())
	}
}

var auditFieldNames = map[string]bool{
	"name": true, "description": true, "brand_color": true, "is_ai_enabled": true,
	"content": true, "title": true, "draft": true, "scheduled_at": true, "target_accounts": true, "target_account_ids": true,
	"account_content_override": true, "media_ids": true, "visibility": true, "versions": true,
	"is_active": true, "feed_url": true, "content_template": true, "title_template": true, "title_hint": true, "output_mode": true, "max_posts_per_day": true, "ai_enhance_enabled": true, "prompt_hint": true, "tonality": true, "initial_sync_mode": true,
	"enabled": true, "recurrence_json": true, "materialize_horizon_days": true, "announcement_enabled": true,
	"provider": true, "model": true, "base_url": true, "api_key": true, "role": true, "filename": true, "retention_days": true,
}

// auditMutation is the database-only boundary for handlers that first call a provider.
func (a *API) auditMutation(r *http.Request, mutate func(*http.Request) error) error {
	state := &auditRequestState{requestID: uuid.NewString()}
	return a.store.WithAuditTransaction(r.Context(), func(ctx context.Context) error {
		err := mutate(r.WithContext(context.WithValue(ctx, auditRequestKey{}, state)))
		if err != nil {
			return err
		}
		return state.err
	})
}

// auditChanged ignores persistence bookkeeping, so retries with identical values
// do not invent another meaningful activity event.
func auditChanged(before, after any) bool {
	normalize := func(value any) map[string]any {
		raw, _ := json.Marshal(value)
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		for _, key := range []string{"updated_at", "created_at", "last_fetched_at", "last_sync_at"} {
			delete(fields, key)
		}
		return fields
	}
	return !reflect.DeepEqual(normalize(before), normalize(after))
}

func auditVersionsChanged(before, after []domain.PostVersion) bool {
	content := func(versions []domain.PostVersion) map[string]string {
		out := map[string]string{}
		for _, version := range versions {
			if version.Content != "" {
				out[version.AccountID] = version.Content
			}
		}
		return out
	}
	return !reflect.DeepEqual(content(before), content(after))
}
