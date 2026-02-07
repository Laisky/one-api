## 2026-01-31 - [User Enumeration and Timing Attack Mitigation in Password Reset]
**Vulnerability:** The password reset endpoint explicitly indicated whether an email was registered and performed network-bound email sending synchronously.
**Learning:** Returning specific error messages for non-existent accounts allows attackers to enumerate users. Synchronous email sending allows timing attacks to distinguish between existing and non-existing accounts.
**Prevention:** Always return a uniform success response for public endpoints that accept identifiers (e.g., password reset, registration). Perform latency-heavy operations like email sending in a background goroutine to ensure consistent response times.

## 2025-05-22 - [SSRF Protection for User-Supplied Content]
**Vulnerability:** The application fetched user-supplied resources (like images) using a standard HTTP client without restricting access to internal network addresses.
**Learning:** Even with a proxy, direct outbound requests from the server to user-controlled URLs pose an SSRF risk. Differentiating between "relay" requests (trusted/admin-configured) and "user-content" requests (untrusted) allows applying stricter security controls where needed without breaking core functionality.
**Prevention:** Implement a custom `net.Dialer.Control` function to block connections to internal/private IP ranges (RFC 1918, etc.) for clients that handle untrusted URLs.
