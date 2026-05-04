package provider

import "context"

type outboundPolicyKey struct{}

// OutboundPolicy controls outbound HTTP safety checks for provider instance URLs.
type OutboundPolicy struct {
	// AllowPrivateLAN skips blocking loopback, RFC1918, link-local, and similar addresses
	// after DNS resolution. Intended for local development and tests only.
	AllowPrivateLAN bool
}

// WithOutboundInstancePolicy attaches policy for normalizeInstanceURL and related checks.
func WithOutboundInstancePolicy(ctx context.Context, p OutboundPolicy) context.Context {
	return context.WithValue(ctx, outboundPolicyKey{}, p)
}

func outboundPolicyFromContext(ctx context.Context) OutboundPolicy {
	if ctx == nil {
		return OutboundPolicy{}
	}
	p, _ := ctx.Value(outboundPolicyKey{}).(OutboundPolicy)
	return p
}
