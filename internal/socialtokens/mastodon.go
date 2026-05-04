package socialtokens

import (
	"context"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
)

// EnsureMastodonFresh refreshes the access token when it is close to expiry and the account uses Mastodon OAuth with a refresh token.
func EnsureMastodonFresh(ctx context.Context, s store.Store, providers *provider.Registry, account domain.SocialAccount) (domain.SocialAccount, error) {
	if account.Provider != "mastodon" {
		return account, nil
	}
	if account.ProviderInstanceID == "" || account.AccessTokenExpiresAt == nil {
		return account, nil
	}
	if time.Until(*account.AccessTokenExpiresAt) > 2*time.Minute {
		return account, nil
	}
	refreshPlain, err := s.DecryptRefreshToken(account)
	if err != nil || refreshPlain == "" {
		return account, nil
	}
	pImpl, ok := providers.Get(account.Provider)
	if !ok {
		return account, nil
	}
	refresher, ok := pImpl.(provider.OAuthTokenRefresher)
	if !ok {
		return account, nil
	}
	instance, err := s.GetProviderInstanceByID(ctx, account.ProviderInstanceID)
	if err != nil {
		return account, err
	}
	clientSecret, err := s.DecryptProviderInstanceClientSecret(instance)
	if err != nil || clientSecret == "" {
		return account, err
	}
	access, newRefresh, exp, err := refresher.RefreshAccessToken(ctx, instance, clientSecret, refreshPlain)
	if err != nil {
		return account, err
	}
	rt := newRefresh
	if rt == "" {
		rt = refreshPlain
	}
	if err := s.UpdateSocialAccountTokens(ctx, account.ID, access, rt, exp); err != nil {
		return account, err
	}
	return s.GetAccountByID(ctx, account.ID)
}
