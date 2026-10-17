package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func TestPushSubscriptionsRequireFullAuthority(t *testing.T) {
	for _, kind := range []string{"reader", "writer", "team bound", "team bound writer", "full", "browser"} {
		for _, route := range []string{"GET /v1/me/push-subscriptions", "POST /v1/me/push-subscriptions", "PATCH /v1/me/push-subscriptions/{subID}", "DELETE /v1/me/push-subscriptions/{subID}", "POST /v1/me/push-subscriptions/{subID}/test"} {
			t.Run(kind+"/"+route, func(t *testing.T) {
				f := newEndpointFixture(t)
				ctx := context.Background()
				sub, err := f.store.CreatePushSubscription(ctx, f.user.ID, domain.PushSubscription{Endpoint: "https://push.example.test/existing", P256dh: "key", Auth: "secret", Enabled: false})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.store.UpdatePushSubscriptionEnabled(ctx, f.user.ID, sub.ID, false); err != nil {
					t.Fatal(err)
				}
				scopes := ""
				var team *string
				switch kind {
				case "reader":
					scopes = `["read"]`
				case "writer":
					scopes = `["read","write","delete"]`
				case "team bound":
					team = &f.team.ID
				case "team bound writer":
					team = &f.team.ID
					scopes = `["read","write","delete"]`
				}
				bearer, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, kind, nil, scopes, team, "")
				if kind == "browser" {
					bearer, _, err = f.store.CreateSessionAPIToken(ctx, f.user.ID, time.Hour)
				}
				if err != nil {
					t.Fatal(err)
				}
				method, path, _ := strings.Cut(route, " ")
				path = strings.ReplaceAll(path, "{subID}", sub.ID)
				req := httptest.NewRequest(method, path, bytes.NewBufferString(`{"endpoint":"https://push.example.test/new","keys":{"p256dh":"key","auth":"secret"},"enabled":true}`))
				if kind == "browser" {
					req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: bearer})
					req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "csrf"})
					req.Header.Set(auth.CSRFHeaderName, "csrf")
				} else {
					req.Header.Set("Authorization", "Bearer "+bearer)
				}
				rec := httptest.NewRecorder()
				f.handler.ServeHTTP(rec, req)
				restricted := kind != "full" && kind != "browser"
				if restricted {
					requireStatus(t, rec, http.StatusForbidden)
					if rec.Header().Get("X-Error-Code") != "scope_required" && !strings.Contains(rec.Body.String(), "scope_required") {
						t.Fatalf("wrong denial: %s", rec.Body.String())
					}
					subs, err := f.store.ListPushSubscriptions(ctx, f.user.ID)
					if err != nil {
						t.Fatal(err)
					}
					if len(subs) != 1 || subs[0].Enabled {
						t.Fatalf("restricted token changed subscriptions: %+v", subs)
					}
				} else {
					want := map[string]int{"GET": http.StatusOK, "POST": http.StatusCreated, "PATCH": http.StatusOK, "DELETE": http.StatusNoContent}[method]
					if strings.HasSuffix(path, "/test") {
						want = http.StatusBadGateway
					}
					requireStatus(t, rec, want)
				}
			})
		}
	}
}
