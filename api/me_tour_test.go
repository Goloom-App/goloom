package api_test

import (
	"net/http"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

// PUT /v1/me/tour persists the guided-tour flag on the authenticated user so
// it follows the account across browsers and devices, and /v1/me reports it.
func TestMeTour_roundTrip(t *testing.T) {
	f := newEndpointFixture(t)

	me := decodeJSON[struct {
		User domain.User `json:"user"`
	}](t, f.do(t, http.MethodGet, "/v1/me", nil))
	if me.User.TourDone {
		t.Fatalf("fresh user must not have tour_done: %+v", me.User)
	}

	rec := f.do(t, http.MethodPut, "/v1/me/tour", map[string]any{"done": true})
	requireStatus(t, rec, http.StatusOK)
	updated := decodeJSON[domain.User](t, rec)
	if !updated.TourDone {
		t.Fatalf("PUT tour done=true: %+v", updated)
	}

	me = decodeJSON[struct {
		User domain.User `json:"user"`
	}](t, f.do(t, http.MethodGet, "/v1/me", nil))
	if !me.User.TourDone {
		t.Fatalf("/v1/me must report tour_done after PUT: %+v", me.User)
	}

	rec = f.do(t, http.MethodPut, "/v1/me/tour", map[string]any{"done": false})
	requireStatus(t, rec, http.StatusOK)
	if reset := decodeJSON[domain.User](t, rec); reset.TourDone {
		t.Fatalf("PUT tour done=false: %+v", reset)
	}
}

func TestMeTour_requiresAuth(t *testing.T) {
	f := newEndpointFixture(t)
	f.bearer = "invalid-token"
	rec := f.do(t, http.MethodPut, "/v1/me/tour", map[string]any{"done": true})
	requireStatus(t, rec, http.StatusUnauthorized)
}
