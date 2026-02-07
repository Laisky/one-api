## 2026-01-31 - [User Enumeration and Timing Attack Mitigation in Password Reset]
**Vulnerability:** The password reset endpoint explicitly indicated whether an email was registered and performed network-bound email sending synchronously.
**Learning:** Returning specific error messages for non-existent accounts allows attackers to enumerate users. Synchronous email sending allows timing attacks to distinguish between existing and non-existing accounts.
**Prevention:** Always return a uniform success response for public endpoints that accept identifiers (e.g., password reset, registration). Perform latency-heavy operations like email sending in a background goroutine to ensure consistent response times.

## 2025-05-22 - [SSRF Protection for User-Supplied Content]
**Vulnerability:** The application fetched user-supplied resources (like images) using a standard HTTP client without restricting access to internal network addresses.
**Learning:** Even with a proxy, direct outbound requests from the server to user-controlled URLs pose an SSRF risk. Differentiating between "relay" requests (trusted/admin-configured) and "user-content" requests (untrusted) allows applying stricter security controls where needed without breaking core functionality.
**Prevention:** Implement a custom `net.Dialer.Control` function to block connections to internal/private IP ranges (RFC 1918, etc.) for clients that handle untrusted URLs.

## 2025-05-22 - [SSRF Protection: Fail-Closed and Error Handling]
**Vulnerability:** Initial SSRF protection implementation had a fail-open gap where unparseable IP addresses were allowed, and it used inconsistent error handling.
**Learning:** Security controls must "fail-closed"—if an input (like an IP address) cannot be validated, access must be denied. Additionally, adhering to project-specific error wrapping guidelines (using `Laisky/errors/v2`) ensures consistent stack traces and contextual information for security events.
**Prevention:** Always ensure security validation logic explicitly handles cases where input parsing or validation fails by denying the request.

## 2025-05-22 - [SSRF Protection: Proxy Handling and DNS Rebinding]
**Vulnerability:** Initial SSRF protection blocked connections to explicitly configured internal proxies and was perceived as potentially vulnerable to DNS rebinding if checks were only done at the URL level.
**Learning:** Using `net.Dialer.Control` is effective against DNS rebinding because it intercepts the actual IP addresses being dialed after resolution. However, when a proxy is used, the dialer connects to the proxy, not the target. Therefore, the configured proxy must be exempted from the internal IP check to avoid breaking legitimate deployments using local egress proxies.
**Prevention:** In `dialer.Control`, check if the dialed address matches the explicitly configured proxy (host or resolved IPs) and allow it if it does.
