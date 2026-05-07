# Security Analysis Report - Goloom

This document lists identified security concerns in the Goloom codebase, along with their ratings and proposed solutions.

## Findings

### 1. Rate Limiter Memory Leak
- **Rating:** Medium
- **Location:** `internal/security/security.go` (`Limiter` struct and `Middleware` method)
- **Description:** The rate limiter uses a `sync.Map` to store state for each visitor (keyed by IP). There is no mechanism to expire or remove old entries. Over time, as unique IP addresses connect to the service, the map will grow indefinitely, potentially leading to memory exhaustion and Denial of Service (DoS).
- **Solution:** Use a cache with a Time-To-Live (TTL) mechanism (e.g., `github.com/patrickmn/go-cache` or a custom implementation with periodic cleanup) instead of a plain `sync.Map`.

### 2. SSRF in Provider Instance Discovery/Registration
- **Rating:** Medium
- **Location:** `internal/provider/mastodon.go` and `internal/provider/utils.go` (`normalizeInstanceURL`)
- **Description:** When an administrator creates or updates a Provider Instance, the system performs outbound HTTP requests to the provided `instance_url` for discovery and application registration. The URL validation does not block internal IP ranges (RFC 1918) or localhost. An authenticated administrator could use this to probe internal network services or access local resources.
- **Solution:** Implement strict URL validation in `normalizeInstanceURL` that prevents requests to private, loopback, or link-local IP addresses, unless explicitly allowed via configuration.

### 3. Trusting Spoofable `X-Forwarded-For` Header
- **Rating:** Low
- **Location:** `internal/security/security.go` (`clientIP` function)
- **Description:** The `clientIP` function trusts the first value in the `X-Forwarded-For` header without verifying if the request came from a trusted proxy. An attacker can easily spoof this header to bypass IP-based rate limiting or mask their true origin in logs.
- **Solution:** Only trust `X-Forwarded-For` if it comes from a known, trusted proxy IP range. Alternatively, provide a configuration option to specify which header to use for the client IP and whether to trust it.

### 4. Potential Open Redirect via Lax OAuth Origin Validation
- **Rating:** Low
- **Location:** `api/oauth.go` (`isAllowedOAuthOrigin`)
- **Description:** The OAuth `return_to` URL is validated against `AllowedOrigins`. If `AllowedOrigins` is configured with `*` (which might be intended only for CORS), the validation effectively allows any origin as a redirect target. This can be exploited for phishing attacks by redirecting users to a malicious site after a successful (or failed) OAuth flow.
- **Solution:** Use a separate, more restrictive whitelist for OAuth redirect origins instead of reusing the CORS `AllowedOrigins` list.

### 5. Use of Insecure Default Encryption Key
- **Rating:** Low
- **Location:** `internal/config/config.go`
- **Description:** The application falls back to a hardcoded "development-insecure-key" if `ENCRYPTION_KEY` is not provided. While it logs a warning (via the encrypter check), it still allows the application to start in a vulnerable state if the user fails to configure it properly in production.
- **Solution:** Require `ENCRYPTION_KEY` to be explicitly set in production environments (e.g., when `APP_ENV=production`). Fail to start if it is missing or insecure.

## Security Architecture Observations

- **Data Encryption:** Sensitive tokens and secrets are encrypted using AES-GCM before being stored in the database, which is a strong practice.
- **Password/Token Hashing:** API tokens and invitation tokens are stored as SHA-256 hashes, preventing plain-text exposure in case of a database leak.
- **CSRF Protection:** The API uses Bearer tokens and does not rely on cookies for authentication, making it inherently resistant to Cross-Site Request Forgery (CSRF).
- **XSS Prevention:** The application uses `bluemonday` for HTML sanitization of user-generated content in posts, reducing the risk of stored XSS.
