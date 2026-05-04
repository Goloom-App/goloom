package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const instanceHostLookupTimeout = 5 * time.Second

func validateInstanceOutboundURL(ctx context.Context, u *url.URL) error {
	if u == nil {
		return errors.New("instance_url is invalid")
	}
	if u.User != nil && u.User.String() != "" {
		return errors.New("instance_url must not contain credentials")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return errors.New("instance_url must include a valid host")
	}

	if outboundPolicyFromContext(ctx).AllowPrivateLAN {
		return nil
	}

	if err := validateResolvedHost(ctx, host); err != nil {
		return err
	}
	return nil
}

func validateResolvedHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if isNonPublicIP(ip) {
			return fmt.Errorf("instance host %s resolves to a non-public address", host)
		}
		return nil
	}

	if isForbiddenHostname(host) {
		return fmt.Errorf("instance host %q is not allowed", host)
	}

	lookupCtx, cancel := context.WithTimeout(ctx, instanceHostLookupTimeout)
	defer cancel()

	resolver := net.DefaultResolver
	addrs, err := resolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("resolve instance host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("instance host %q has no IP addresses", host)
	}
	for _, addr := range addrs {
		if ip := addr.IP; isNonPublicIP(ip) {
			return fmt.Errorf("instance host %q resolves to non-public address %s", host, ip)
		}
	}
	return nil
}

func isForbiddenHostname(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	switch h {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	default:
		return false
	}
}

func isNonPublicIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}
