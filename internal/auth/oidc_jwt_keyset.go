package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

// hybridJWTKeySet verifies ID tokens that use either JWKS-backed asymmetric algorithms
// (RS256, ES256, …) or HMAC (HS256/384/512) with the OAuth2 client secret.
// Some IdPs (e.g. Authentik with symmetric signing) issue HS256 id_tokens; go-oidc's
// default remote JWKS set cannot verify those.
type hybridJWTKeySet struct {
	remote       oidc.KeySet
	clientSecret string
}

func newHybridJWTKeySet(remote oidc.KeySet, clientSecret string) oidc.KeySet {
	return &hybridJWTKeySet{
		remote:       remote,
		clientSecret: strings.TrimSpace(clientSecret),
	}
}

func (h *hybridJWTKeySet) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	alg, err := peekJWTSigningAlg(jwt)
	if err != nil {
		return nil, err
	}
	switch alg {
	case string(jose.HS256), string(jose.HS384), string(jose.HS512):
		if h.clientSecret == "" {
			return nil, fmt.Errorf("id token uses %s but OIDC_CLIENT_SECRET is empty", alg)
		}
		jws, err := jose.ParseSigned(jwt, []jose.SignatureAlgorithm{jose.SignatureAlgorithm(alg)})
		if err != nil {
			return nil, fmt.Errorf("parse id_token: %w", err)
		}
		payload, err := jws.Verify([]byte(h.clientSecret))
		if err != nil {
			return nil, fmt.Errorf("verify id_token HMAC: %w", err)
		}
		return payload, nil
	default:
		return h.remote.VerifySignature(ctx, jwt)
	}
}

func peekJWTSigningAlg(compactJWT string) (string, error) {
	parts := strings.Split(strings.TrimSpace(compactJWT), ".")
	if len(parts) < 1 {
		return "", errors.New("jwt: malformed (no segments)")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("jwt: decode header: %w", err)
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(raw, &hdr); err != nil {
		return "", fmt.Errorf("jwt: decode header json: %w", err)
	}
	if hdr.Alg == "" {
		return "", errors.New("jwt: missing alg header")
	}
	return hdr.Alg, nil
}

type oidcDiscoveryFields struct {
	Issuer                           string   `json:"issuer"`
	JWKSURI                          string   `json:"jwks_uri"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
}

func buildVerifierFromProvider(ctx context.Context, provider *oidc.Provider, clientID, clientSecret string) (*oidc.IDTokenVerifier, error) {
	var disc oidcDiscoveryFields
	if err := provider.Claims(&disc); err != nil {
		return nil, fmt.Errorf("oidc discovery decode: %w", err)
	}
	if strings.TrimSpace(disc.Issuer) == "" || strings.TrimSpace(disc.JWKSURI) == "" {
		return nil, fmt.Errorf("oidc discovery missing issuer or jwks_uri")
	}

	remote := oidc.NewRemoteKeySet(ctx, strings.TrimSpace(disc.JWKSURI))
	keySet := newHybridJWTKeySet(remote, clientSecret)

	supported := buildSupportedIDTokenAlgorithms(disc.IDTokenSigningAlgValuesSupported, clientSecret)
	cfg := &oidc.Config{
		ClientID:             strings.TrimSpace(clientID),
		SupportedSigningAlgs: supported,
	}
	return oidc.NewVerifier(strings.TrimSpace(disc.Issuer), keySet, cfg), nil
}

// union of algorithms Discovery allows and goloom accepts; HS* added when a client secret
// is configured so IdPs may still choose symmetric signing without advertising it inconsistently.
func buildSupportedIDTokenAlgorithms(discovered []string, clientSecret string) []string {
	asymmetricOK := map[string]bool{
		oidc.RS256: true, oidc.RS384: true, oidc.RS512: true,
		oidc.ES256: true, oidc.ES384: true, oidc.ES512: true,
		oidc.PS256: true, oidc.PS384: true, oidc.PS512: true,
		oidc.EdDSA: true,
	}
	symmetricOK := map[string]bool{
		string(jose.HS256): true,
		string(jose.HS384): true,
		string(jose.HS512): true,
	}

	seen := make(map[string]bool)
	var ordered []string
	add := func(alg string) {
		a := strings.TrimSpace(alg)
		if a == "" || seen[a] {
			return
		}
		if asymmetricOK[a] || (symmetricOK[a] && strings.TrimSpace(clientSecret) != "") {
			seen[a] = true
			ordered = append(ordered, a)
		}
	}

	for _, a := range discovered {
		add(a)
	}
	if len(ordered) == 0 {
		add(oidc.RS256)
	}
	if strings.TrimSpace(clientSecret) != "" {
		for _, a := range []string{string(jose.HS256), string(jose.HS384), string(jose.HS512)} {
			add(a)
		}
	}
	return ordered
}
