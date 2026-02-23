## 2026-01-31 - [User Enumeration and Timing Attack Mitigation in Password Reset]

**Vulnerability:** The password reset endpoint explicitly indicated whether an email was registered and performed network-bound email sending synchronously.
**Learning:** Returning specific error messages for non-existent accounts allows attackers to enumerate users. Synchronous email sending allows timing attacks to distinguish between existing and non-existing accounts.
**Prevention:** Always return a uniform success response for public endpoints that accept identifiers (e.g., password reset, registration). Perform latency-heavy operations like email sending in a background goroutine to ensure consistent response times.
