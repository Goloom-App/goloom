// Package netguard keeps outbound HTTP calls to user-supplied URLs away from
// internal networks (SSRF). A Policy validates URLs up front and guards the
// dialer, so a host that resolves differently at call time (DNS rebinding)
// still cannot reach a non-public address. Admins may allowlist specific
// hosts (e.g. a local Ollama) that are then dialed without the address check.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Resolver is the DNS subset the guard needs; tests inject a fake.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// DialFunc matches http.Transport.DialContext.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// Policy decides which outbound hosts may be contacted.
type Policy struct {
	// allowed holds allowlist keys: "host" permits every port, "host:port"
	// (net.JoinHostPort form) only that port.
	allowed  map[string]struct{}
	resolver Resolver
}

// NewPolicy builds a policy that only permits public addresses, plus the given
// allowlist entries regardless of what they resolve to. An entry is a hostname
// or IP literal (case-insensitive), optionally with a port ("localhost:11434")
// to restrict it to that port.
func NewPolicy(allowedHosts []string) Policy {
	allowed := make(map[string]struct{}, len(allowedHosts))
	for _, entry := range allowedHosts {
		entry = strings.TrimSpace(entry)
		if host, port, err := net.SplitHostPort(entry); err == nil {
			if h := normalizeHost(host); h != "" && port != "" {
				allowed[net.JoinHostPort(h, port)] = struct{}{}
			}
			continue
		}
		if h := normalizeHost(entry); h != "" {
			allowed[h] = struct{}{}
		}
	}
	return Policy{allowed: allowed, resolver: net.DefaultResolver}
}

func normalizeHost(h string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(h), "[]"))
}

func (p Policy) allows(host, port string) bool {
	h := normalizeHost(host)
	if h == "" {
		return false
	}
	if _, ok := p.allowed[h]; ok {
		return true
	}
	_, ok := p.allowed[net.JoinHostPort(h, port)]
	return ok
}

// nonPublicPrefixes lists special-use ranges the stdlib predicates in
// IsPublicAddr do not cover.
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT shared address space
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("192.88.99.0/24"),  // 6to4 relay anycast (deprecated)
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved (incl. broadcast)
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use IPv4/IPv6 translation
	netip.MustParsePrefix("2001:db8::/32"),   // IPv6 documentation
	netip.MustParsePrefix("2001::/23"),       // reserved protocol assignments (incl. benchmarking)
	netip.MustParsePrefix("2001:ffff::/32"),  // reserved (deprecated overlay anycast)
	netip.MustParsePrefix("2002::/16"),       // 6to4 (deprecated)
	netip.MustParsePrefix("0100::/64"),       // discard-only
	netip.MustParsePrefix("3fff::/20"),       // reserved for future documentation
	netip.MustParsePrefix("fec0::/10"),       // site-local (deprecated)
}

// embeddedIPv4Prefixes are IPv6 ranges whose last 32 bits carry an IPv4
// address that a translator or the stack may route to: the address must be
// judged by that IPv4 value, or 64:ff9b::7f00:1 reaches 127.0.0.1 via NAT64.
var embeddedIPv4Prefixes = []netip.Prefix{
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 well-known prefix
	netip.MustParsePrefix("::ffff:0:0:0/96"), // SIIT IPv4-translated
	netip.MustParsePrefix("::/96"),           // IPv4-compatible (deprecated)
}

// IsPublicAddr reports whether addr is a routable public address. Loopback,
// RFC1918/ULA private ranges, link-local (incl. cloud metadata), multicast,
// unspecified and the reserved ranges in nonPublicPrefixes are not.
func IsPublicAddr(addr netip.Addr) bool {
	// Unmap first so IPv4-in-IPv6 tuples are judged by their IPv4 value.
	addr = addr.Unmap()
	if addr.Is6() {
		for _, p := range embeddedIPv4Prefixes {
			if p.Contains(addr) {
				b := addr.As16()
				return IsPublicAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
			}
		}
	}
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsLoopback() ||
		addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return false
	}
	for _, p := range nonPublicPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return addr.IsGlobalUnicast()
}

// publicAddrs resolves host and fails unless every address is public, so a
// host with one internal record can never be used.
func (p Policy) publicAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	var addrs []netip.Addr
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		addrs = []netip.Addr{addr}
	} else {
		if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
			return nil, fmt.Errorf("host %q is not allowed", host)
		}
		addrs, err = p.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve host %q: %w", host, err)
		}
		if len(addrs) == 0 {
			return nil, fmt.Errorf("host %q has no addresses", host)
		}
	}
	for _, a := range addrs {
		if !IsPublicAddr(a) {
			return nil, fmt.Errorf("host %q resolves to non-public address %s", host, a)
		}
	}
	return addrs, nil
}

// CheckURL validates a user-supplied base URL: http(s), a host, no embedded
// credentials, and a public address unless the host is allowlisted.
func (p Policy) CheckURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url must use http or https")
	}
	if u.User != nil {
		return errors.New("url must not contain credentials")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("url must include a host")
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	if p.allows(host, port) {
		return nil
	}
	_, err = p.publicAddrs(ctx, host)
	return err
}

// DialContext wraps base so non-allowlisted hosts are resolved once and dialed
// by IP only when every resolved address is public.
func (p Policy) DialContext(base DialFunc) DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("netguard: invalid address %q: %w", addr, err)
		}
		if p.allows(host, port) {
			return base(ctx, network, addr)
		}
		addrs, err := p.publicAddrs(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("netguard: refusing %q: %w", addr, err)
		}
		return base(ctx, network, net.JoinHostPort(addrs[0].String(), port))
	}
}

// HTTPClient returns a client whose connections and redirects obey the policy.
// Proxies from the environment are ignored: a proxy would hide the real
// target from the dial check.
func (p Policy) HTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = p.DialContext(dialer.DialContext)
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return p.CheckURL(req.Context(), req.URL.String())
		},
	}
}
