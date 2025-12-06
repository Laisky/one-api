# API Security Audit Report

## Executive Summary

This comprehensive security audit examines all API endpoints in the One API system, focusing on authentication mechanisms, authorization controls, billing security, and potential attack vectors. The audit identifies critical security vulnerabilities and provides actionable recommendations.

## Methodology

The audit analyzed:

- All API endpoints defined in `router/api.go` and `router/relay.go`
- Authentication middleware implementations
- Role-based access control mechanisms
- Billing and quota management systems
- Rate limiting and DDoS protection
- Input validation and data security measures

## API Endpoint Inventory

### Public Endpoints (No Authentication Required)

```
GET  /api/status                    - System status
GET  /api/notice                    - Public notices
GET  /api/about                     - System information
GET  /api/home_page_content         - Homepage content
GET  /cost/request/:request_id      - Request cost lookup (⚠️ SECURITY RISK)
```

### Authentication Endpoints (Rate Limited)

```
POST /api/user/register             - User registration (CriticalRateLimit + Turnstile)
POST /api/user/login                - User login (CriticalRateLimit)
GET  /api/user/logout               - User logout
POST /api/user/reset                - Password reset (CriticalRateLimit)
GET  /api/verification              - Email verification (CriticalRateLimit + Turnstile)
GET  /api/reset_password            - Password reset email (CriticalRateLimit + Turnstile)
```

### OAuth Endpoints (Rate Limited)

```
GET  /api/oauth/github              - GitHub OAuth (CriticalRateLimit)
GET  /api/oauth/oidc                - OIDC authentication (CriticalRateLimit)
GET  /api/oauth/lark                - Lark OAuth (CriticalRateLimit)
GET  /api/oauth/state               - OAuth state generation (CriticalRateLimit)
GET  /api/oauth/wechat              - WeChat authentication (CriticalRateLimit)
GET  /api/oauth/wechat/bind         - WeChat binding (CriticalRateLimit + UserAuth)
GET  /api/oauth/email/bind          - Email binding (CriticalRateLimit + UserAuth)
```

### User-Level Endpoints (UserAuth Required)

```
GET  /api/models                    - List available models
GET  /api/models/display            - Model display information
GET  /api/user/dashboard            - User dashboard data
GET  /api/user/dashboard/users      - User list (⚠️ ROOT ONLY)
GET  /api/user/self                 - User profile
PUT  /api/user/self                 - Update profile
DELETE /api/user/self               - Delete account
GET  /api/user/token                - Generate access token
GET  /api/user/aff                  - Affiliate code
POST /api/user/topup                - User top-up
GET  /api/user/available_models     - Available models
GET  /api/user/totp/status          - TOTP status
GET  /api/user/totp/setup           - TOTP setup
POST /api/user/totp/confirm         - TOTP confirmation
POST /api/user/totp/disable         - TOTP disable
```

### Token Management (UserAuth Required)

```
GET  /api/token/                    - List user tokens
GET  /api/token/search              - Search tokens
GET  /api/token/:id                 - Get token details
POST /api/token/                    - Create token
PUT  /api/token/                    - Update token
DELETE /api/token/:id               - Delete token
POST /api/token/consume             - Token consumption (TokenAuth)
```

### Log Access (Mixed Permissions)

```
GET  /api/log/                      - All logs (AdminAuth)
DELETE /api/log/                    - Delete logs (AdminAuth)
GET  /api/log/stat                  - Log statistics (AdminAuth)
GET  /api/log/self/stat             - User log stats (UserAuth)
GET  /api/log/search                - Search all logs (AdminAuth)
GET  /api/log/self                  - User logs (UserAuth)
GET  /api/log/self/search           - Search user logs (UserAuth)
```

### Admin-Level Endpoints (AdminAuth Required)

```
POST /api/topup                     - Admin top-up operations
GET  /api/user/                     - List all users
GET  /api/user/search               - Search users
GET  /api/user/:id                  - Get user details
POST /api/user/                     - Create user
POST /api/user/manage               - Manage user (enable/disable/promote/demote)
PUT  /api/user/                     - Update user
DELETE /api/user/:id                - Delete user
POST /api/user/totp/disable/:id     - Admin disable user TOTP
```

### Channel Management (AdminAuth Required)

```
GET  /api/channel/                  - List channels
GET  /api/channel/search            - Search channels
GET  /api/channel/models            - List all models
GET  /api/channel/:id               - Get channel
GET  /api/channel/test              - Test all channels
GET  /api/channel/test/:id          - Test specific channel
GET  /api/channel/pricing/:id       - Get channel pricing
GET  /api/channel/default-pricing   - Get default pricing
POST /api/channel/                  - Add channel
PUT  /api/channel/                  - Update channel
PUT  /api/channel/pricing/:id       - Update channel pricing
DELETE /api/channel/disabled        - Delete disabled channels
DELETE /api/channel/:id             - Delete channel
```

### Debug Endpoints (AdminAuth Required)

```
POST /api/debug/channel/:id/debug   - Debug channel configs
GET  /api/debug/channels            - Debug all channels
POST /api/debug/channel/:id/fix     - Fix channel configs
GET  /api/debug/channels/validate   - Validate all channels
POST /api/debug/channels/remigrate  - Re-migrate all channels
GET  /api/debug/channel/:id/migration-status - Migration status
POST /api/debug/channels/clean      - Clean mixed model data
```

### Redemption Management (AdminAuth Required)

```
GET  /api/redemption/               - List redemptions
GET  /api/redemption/search         - Search redemptions
GET  /api/redemption/:id            - Get redemption
POST /api/redemption/               - Create redemption
PUT  /api/redemption/               - Update redemption
DELETE /api/redemption/:id          - Delete redemption
```

### Group Management (AdminAuth Required)

```
GET  /api/group/                    - Get groups
```

### Root-Level Endpoints (RootAuth Required)

```
GET  /api/option/                   - Get system options
PUT  /api/option/                   - Update system options
```

### AI API Endpoints (TokenAuth Required)

```
GET  /v1/models                     - List models
GET  /v1/models/:model              - Get model details
POST /v1/chat/completions           - Chat completions
POST /v1/completions                - Text completions
POST /v1/embeddings                 - Embeddings
POST /v1/moderations                - Content moderation
POST /v1/images/generations         - Image generation
POST /v1/audio/speech               - Text-to-speech
POST /v1/audio/transcriptions       - Speech-to-text
POST /v1/audio/translations         - Audio translation
```

## Role Hierarchy Analysis

The system implements a three-tier role hierarchy:

```
RoleGuestUser   = 0   (Not used in practice)
RoleCommonUser  = 1   (Regular users)
RoleAdminUser   = 10  (Administrators)
RoleRootUser    = 100 (Super administrators)
```

### Permission Levels:

- **UserAuth**: Allows RoleCommonUser (1) and above
- **AdminAuth**: Allows RoleAdminUser (10) and above
- **RootAuth**: Allows RoleRootUser (100) only
- **TokenAuth**: API token-based authentication with additional restrictions

## Critical Security Vulnerabilities

### 🔴 HIGH RISK

#### 1. Unprotected Cost Endpoint

**Endpoint**: `GET /cost/request/:request_id`
**Risk**: Information disclosure, billing data exposure
**Details**: This endpoint has NO authentication requirements, allowing anyone to query request costs by guessing request IDs.

#### 2. Privilege Escalation in User Management

**Location**: `controller/user.go:ManageUser()`
**Risk**: Privilege escalation
**Details**: Admin users can promote other users to admin level, but the check `myRole <= user.Role && myRole != model.RoleRootUser` has a logical flaw that could allow admins to promote users with equal roles.

#### 3. Debug Endpoints Expose Sensitive Data

**Endpoints**: All `/api/debug/*` endpoints
**Risk**: Information disclosure, system manipulation
**Details**: Debug endpoints can expose channel configurations, API keys, and allow system-wide data manipulation with only AdminAuth protection.

### 🟡 MEDIUM RISK

#### 4. Weak Rate Limiting on Critical Operations

**Location**: Various endpoints with `CriticalRateLimit()`
**Risk**: Brute force attacks, account enumeration
**Details**: Rate limiting may be insufficient for preventing sophisticated attacks, especially with distributed sources.

#### 5. CORS Configuration Too Permissive

**Location**: `middleware/cors.go`
**Risk**: Cross-origin attacks
**Details**: `AllowAllOrigins = true` and `AllowHeaders = []string{"*"}` are overly permissive.

#### 6. Token Channel Specification Bypass

**Location**: `middleware/auth.go:TokenAuth()`
**Risk**: Unauthorized channel access
**Details**: Admin users can specify channels via token format `token:channel_id`, but validation may be insufficient.

### 🟢 LOW RISK

#### 7. Information Leakage in Error Messages

**Risk**: Information disclosure
**Details**: Some error messages may reveal system internals or user existence.

#### 8. Session Management

**Risk**: Session fixation, insufficient session security
**Details**: Session handling could be strengthened with additional security measures.

## Billing Security Analysis

### Quota Management Vulnerabilities

#### 1. Race Conditions in Billing

**Location**: `relay/billing/billing.go`
**Risk**: Financial loss through double-spending
**Details**: Pre-consumption and post-consumption billing logic may be vulnerable to race conditions in high-concurrency scenarios.

#### 2. Negative Quota Handling

**Location**: `model/token.go:PreConsumeTokenQuota()`
**Risk**: Quota bypass
**Details**: While there are checks for negative quotas, edge cases in error handling might allow quota bypass.

#### 3. Unlimited Quota Token Abuse

**Location**: Token management system
**Risk**: Resource abuse
**Details**: Unlimited quota tokens could be abused if not properly monitored and restricted.

### Billing Calculation Security

The system uses a complex three-layer pricing model:

1. Channel-specific overrides (highest priority)
2. Adapter default pricing (second priority)
3. Global pricing fallback (third priority)
4. Final default (lowest priority)

**Potential Issues**:

- Pricing manipulation through channel configuration
- Fallback pricing may be exploited
- Model ratio calculations could overflow or underflow

## Rate Limiting Assessment

### Current Implementation

- **Global API Rate Limit**: 480 requests per 3 minutes
- **Global Web Rate Limit**: 240 requests per 3 minutes
- **Critical Rate Limit**: Applied to sensitive operations
- **Channel Rate Limit**: Optional per-channel limiting

### Vulnerabilities

1. **Distributed Attack Bypass**: Rate limiting by IP can be bypassed using distributed sources
2. **Token-based Rate Limiting**: Uses hashed tokens but may be vulnerable to hash collisions
3. **Redis Dependency**: Rate limiting fails open if Redis is unavailable

## Input Validation Analysis

### Strengths

- JSON schema validation using struct tags
- GORM model validation
- Turnstile CAPTCHA on critical endpoints

### Weaknesses

- Insufficient input sanitization in some endpoints
- Potential SQL injection through dynamic queries
- File upload validation may be insufficient

## Recommendations

### Immediate Actions (Critical)

1. **Secure Cost Endpoint**: Add authentication to `/cost/request/:request_id`
2. **Fix Privilege Escalation**: Strengthen role validation in user management
3. **Restrict Debug Endpoints**: Add additional authorization or remove from production
4. **Implement Request ID Validation**: Use UUIDs and validate ownership for cost queries

### Short-term Improvements (High Priority)

1. **Enhance Rate Limiting**: Implement more sophisticated rate limiting with multiple factors
2. **Strengthen CORS Policy**: Restrict origins and headers to necessary values only
3. **Add Audit Logging**: Implement comprehensive audit logging for all admin operations
4. **Improve Error Handling**: Sanitize error messages to prevent information leakage

### Long-term Security Enhancements

1. **Implement API Versioning**: Add proper API versioning for security updates
2. **Add Request Signing**: Implement request signing for critical operations
3. **Enhanced Monitoring**: Add real-time security monitoring and alerting
4. **Regular Security Audits**: Establish periodic security review processes

## Detailed Vulnerability Analysis

### Authentication Bypass Scenarios

#### Scenario 1: Cost Endpoint Exploitation

```bash
# Attacker can enumerate request costs without authentication
curl "https://oneapi.laisky.com/cost/request/req_123456789"
curl "https://oneapi.laisky.com/cost/request/req_123456790"
# ... continue enumeration to gather billing intelligence
```

**Impact**: Competitors could gather pricing intelligence, users could discover system usage patterns.

#### Scenario 2: Session vs Token Auth Confusion

**Location**: `middleware/auth.go:authHelper()`
**Issue**: The fallback from session to token authentication could be exploited if session validation is bypassed.

### Privilege Escalation Attack Vectors

#### Vector 1: Admin Role Promotion Logic Flaw

```go
// Vulnerable code in controller/user.go:ManageUser()
if myRole <= user.Role && myRole != model.RoleRootUser {
    // This check has a logical flaw
    return "No permission"
}
```

**Exploitation**: An admin (role=10) could potentially promote another admin (role=10) because `10 <= 10` is true, but the root user check might not prevent this in all cases.

#### Vector 2: Token Channel Specification Abuse

```bash
# Admin can specify channels via token format
Authorization: Bearer sk-token123-channel456
```

**Risk**: If channel validation is insufficient, admins might access unauthorized channels or bypass channel restrictions.

### Billing Manipulation Scenarios

#### Scenario 1: Race Condition in Quota Consumption

**Location**: `relay/billing/billing.go:PostConsumeQuotaDetailed()`
**Attack**: Concurrent requests could exploit timing windows between pre-consumption and post-consumption billing.

#### Scenario 2: Negative Quota Injection

**Location**: `model/token.go:PostConsumeTokenQuota()`
**Attack**: Manipulating quota values to create negative consumption, potentially increasing available quota.

#### Scenario 3: Model Ratio Manipulation

**Location**: Channel pricing configuration
**Attack**: Admins could set extremely low model ratios to reduce costs artificially.

### DDoS and Resource Exhaustion Vectors

#### Vector 1: Rate Limit Bypass via Distribution

- Use multiple IP addresses to bypass IP-based rate limiting
- Exploit rate limit key generation weaknesses
- Target endpoints with insufficient rate limiting

#### Vector 2: Resource Exhaustion via Debug Endpoints

- Trigger expensive debug operations repeatedly
- Cause system-wide migrations or validations
- Exhaust database connections through debug queries

#### Vector 3: Billing System Overload

- Submit numerous concurrent requests to trigger billing calculations
- Exploit complex pricing calculations to cause CPU exhaustion
- Target quota management operations

### Data Exposure Risks

#### Risk 1: Channel Configuration Exposure

**Endpoints**: `/api/debug/channels`, `/api/channel/pricing/:id`
**Data at Risk**: API keys, model configurations, pricing information

#### Risk 2: User Enumeration

**Endpoints**: User management endpoints
**Attack**: Enumerate valid usernames through error message differences

#### Risk 3: System Configuration Disclosure

**Endpoints**: `/api/option/` (Root only, but still risky)
**Data at Risk**: System secrets, configuration details

## Advanced Attack Scenarios

### Multi-Stage Attack: Admin Account Takeover

1. **Reconnaissance**: Use unprotected cost endpoint to gather system intelligence
2. **User Enumeration**: Exploit user management endpoints to identify admin accounts
3. **Credential Attack**: Use gathered intelligence to target admin accounts
4. **Privilege Abuse**: Once admin access is gained, exploit debug endpoints and billing manipulation

### Supply Chain Attack: Channel Compromise

1. **Channel Creation**: Create malicious channels with manipulated pricing
2. **Model Ratio Manipulation**: Set extremely favorable pricing ratios
3. **Traffic Redirection**: Route high-value requests through compromised channels
4. **Data Harvesting**: Collect sensitive request/response data

### Financial Attack: Quota Manipulation

1. **Token Creation**: Create tokens with specific configurations
2. **Race Condition Exploitation**: Exploit billing race conditions
3. **Negative Quota Injection**: Manipulate quota calculations
4. **Resource Theft**: Consume services without proper billing

## Security Control Effectiveness Assessment

### Authentication Controls: ⚠️ MODERATE

- **Strengths**: Multi-factor authentication support, role-based access
- **Weaknesses**: Session/token confusion, insufficient validation

### Authorization Controls: ⚠️ MODERATE

- **Strengths**: Clear role hierarchy, endpoint-level protection
- **Weaknesses**: Privilege escalation risks, insufficient granularity

### Rate Limiting: ⚠️ MODERATE

- **Strengths**: Multiple rate limiting layers, Redis-backed storage
- **Weaknesses**: Bypass potential, fail-open behavior

### Input Validation: ⚠️ WEAK

- **Strengths**: Basic struct validation, CAPTCHA protection
- **Weaknesses**: Insufficient sanitization, potential injection risks

### Audit Logging: ⚠️ WEAK

- **Strengths**: Basic request logging
- **Weaknesses**: Insufficient security event logging, no real-time monitoring

### Data Protection: ⚠️ WEAK

- **Strengths**: Password hashing, session management
- **Weaknesses**: Sensitive data exposure, insufficient encryption

## Compliance and Regulatory Considerations

### GDPR Implications

- User data deletion capabilities exist but may be insufficient
- Data processing logging may not meet GDPR requirements
- Right to data portability not clearly implemented

### PCI DSS Considerations (if handling payments)

- Payment data handling through top-up functionality
- Insufficient audit logging for financial transactions
- Network security controls may be inadequate

### SOC 2 Readiness

- Access controls partially implemented
- Monitoring and logging insufficient
- Change management processes unclear

## Conclusion

The One API system has a solid foundation with role-based access control and rate limiting, but contains several critical vulnerabilities that require immediate attention. The most serious issues involve unprotected endpoints, privilege escalation risks, and potential billing manipulation. Implementing the recommended fixes will significantly improve the system's security posture.

**Risk Rating**: HIGH - Immediate action required to address critical vulnerabilities
**Recommended Timeline**:

- Critical fixes: Within 1 week
- High priority improvements: Within 1 month
- Long-term enhancements: Within 3 months
