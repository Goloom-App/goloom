package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestPublicationRejectsAreDistinguishableFromUncertainOutcomes(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusUnprocessableEntity, http.StatusRequestTimeout, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		for _, p := range []SocialMediaProvider{NewMastodonProvider(MastodonRegistrationConfig{}), NewBlueskyProvider(), NewFriendicaProvider()} {
			t.Run(p.Name()+http.StatusText(status), func(t *testing.T) {
				_, err := p.Publish(context.Background(), domain.SocialAccount{InstanceURL: server.URL, AuthType: domain.AccountAuthTypeOAuthToken}, PublishAuth{AccessToken: "test"}, PublishRequest{Content: "test"})
				var rejected *PublicationRejectedError
				want := status >= 400 && status < 500 && status != http.StatusRequestTimeout
				if errors.As(err, &rejected) != want {
					t.Fatalf("status %d: rejected=%v, err=%v", status, !want, err)
				}
			})
		}
		server.Close()
	}
}
