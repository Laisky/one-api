package middleware

import (
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/relay/model"
)

// AbortWithError aborts the request with an error message
func AbortWithError(c *gin.Context, statusCode int, err error) {
	logger := gmw.GetLogger(c)
	fields := []zap.Field{
		zap.Int("status_code", statusCode),
		zap.String("error_kind", errkind.Of(err).String()),
	}
	// Identity the bound logger does not already carry: late gin-context values
	// and identity tagged onto the error deep in model/, where the entity struct
	// was in hand. Duplicates of already-bound fields are suppressed.
	fields = append(fields, identity.ExtraFields(c, err)...)

	// zap.Error is attached per branch, never to the shared slice: it renders the
	// full errors/v2 stack, which must not appear on a WARN line.
	if shouldLogAsWarning(statusCode, err) {
		logger.Warn("server abort", append(fields, zap.String("error", errMessage(err)))...)
	} else {
		logger.Error("server abort", append(fields, zap.Error(err))...)
	}

	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": helper.MessageWithRequestId(err.Error(), c.GetString(helper.RequestIdKey)),
			"type":    string(model.ErrorTypeOneAPI),
		},
	})
	c.Abort()
}

// TokenInfo holds information about an API token for logging purposes.
// All fields are masked or reference-based to avoid exposing sensitive data.
type TokenInfo struct {
	MaskedKey   string // Masked API key (prefix...suffix)
	RequestedAt string // Request model (optional)

	// Token identifies the API key by id + uuid + name. The uuid and name are
	// what the web UI shows, so they are what an operator can search on.
	Token identity.TokenRef
	// User identifies the token owner by id + uuid + username. The username is
	// only known once the owner row has been loaded.
	User identity.UserRef
}

// AbortWithTokenError aborts the request with an error message and logs detailed token information.
// This function should be used when rejecting API key requests to provide more context in logs
// for debugging purposes. The token information is safely masked to avoid exposing sensitive data.
func AbortWithTokenError(c *gin.Context, statusCode int, err error, tokenInfo *TokenInfo) {
	logger := gmw.GetLogger(c)
	logFields := []zap.Field{
		zap.Int("status_code", statusCode),
		zap.String("error_kind", errkind.Of(err).String()),
	}

	// Add token info fields if available. The refs carry uuid + name next to the
	// integer ids, because the web UI only exposes uuid and name.
	if tokenInfo != nil {
		logFields = append(logFields, zap.String("api_key", tokenInfo.MaskedKey))
		logFields = tokenInfo.Token.AppendZap(logFields)
		logFields = tokenInfo.User.AppendZap(logFields)
		if tokenInfo.RequestedAt != "" {
			logFields = append(logFields, zap.String("requested_model", tokenInfo.RequestedAt))
		}
	}
	logFields = append(logFields, identity.Fields(err)...)

	// zap.Error only on the ERROR branch: it renders the full errors/v2 stack.
	if shouldLogAsWarning(statusCode, err) {
		logger.Warn("server abort", append(logFields, zap.String("error", errMessage(err)))...)
	} else {
		logger.Error("server abort", append(logFields, zap.Error(err))...)
	}

	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": helper.MessageWithRequestId(err.Error(), c.GetString(helper.RequestIdKey)),
			"type":    string(model.ErrorTypeOneAPI),
		},
	})
	c.Abort()
}

// errMessage renders an error for a WARN log line: the message only, never the
// stack. zap.Error would attach the full errors/v2 trace, which is reserved for
// genuine server faults.
//
// Parameters:
//   - err: the error to render; nil yields an empty string.
//
// Return values:
//   - string: err.Error(), or "" when err is nil.
func errMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// shouldLogAsWarning determines whether an abort should be logged as WARN.
//
// Parameters:
//   - statusCode: HTTP status code returned to client.
//   - err: the error that triggers abort.
//
// Returns:
//   - true if this is a client-caused or intentionally ignored case.
//   - false if this is a server-side failure that should be logged as ERROR.
func shouldLogAsWarning(statusCode int, err error) bool {
	// A recorded fault attribution wins over the transport in BOTH directions.
	// Notably it is what stops a database outage on the auth path — which this
	// middleware reports as HTTP 401 regardless of cause — from being logged at
	// WARN and paging nobody.
	if kind := errkind.Of(err); kind != errkind.Unknown {
		return kind.IsClient()
	}

	if statusCode >= 400 && statusCode < 500 {
		return true
	}

	if err == nil {
		return false
	}

	// Deprecated fallback for errors that have not been marked yet. Remove once
	// every producer on these paths calls errkind.

	switch {
	case strings.Contains(err.Error(), "token not found for key:"):
		return true
	case strings.Contains(err.Error(), "No available channels for Model"):
		// No channel is configured/available for the requested model under the
		// group. This is an operator/configuration condition rather than a
		// server fault, so it is logged as WARN to avoid noisy ERROR alerts.
		return true
	default:
		return false
	}
}

func getRequestModel(c *gin.Context) (string, error) {
	// Realtime WS uses model in query string
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		m := c.Query("model")
		if m == "" {
			return "", errors.New("missing required query parameter: model")
		}
		return m, nil
	}

	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		return "", errors.Wrap(err, "common.UnmarshalBodyReusable failed")
	}

	switch {
	case strings.HasPrefix(c.Request.URL.Path, "/v1/moderations"):
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	case strings.HasSuffix(c.Request.URL.Path, "embeddings"):
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	case strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations"),
		strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits"):
		if modelRequest.Model == "" {
			modelRequest.Model = "dall-e-2"
		}
	case strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions"),
		strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations"):
		if modelRequest.Model == "" {
			modelRequest.Model = "whisper-1"
		}
	}

	return modelRequest.Model, nil
}

// IsModelInList reports whether modelName is permitted by an API token's
// comma-separated allow-list (Token.Models).
//
// This is the authoritative predicate: TokenAuth calls it to 403 a disallowed
// model, and controller.ListModels / controller.RetrieveModel call it to decide
// what a token may see. Both MUST use this one function -- if discovery used a
// different rule, a key would either be shown models it cannot call or denied
// models it can.
//
// Matching is exact: byte-for-byte, case-sensitive, no wildcards, and entries are
// NOT trimmed. A stored list of "a, b" therefore permits "a" and " b", not "b".
// That is long-standing behavior and is deliberately preserved here; changing it
// would silently widen every existing token's permissions.
func IsModelInList(modelName string, models string) bool {
	modelList := strings.Split(models, ",")
	return slices.Contains(modelList, modelName)
}

// authTokenSource identifies which request header (or transport) supplied the
// API credential. It is used purely for diagnostics and never carries the
// credential value itself, so it is safe to log.
type authTokenSource string

const (
	authSourceNone          authTokenSource = "none"
	authSourceAuthorization authTokenSource = "authorization"
	authSourceXAPIKey       authTokenSource = "x-api-key"
	authSourceAPIKey        authTokenSource = "api-key"
	authSourceWebSocket     authTokenSource = "websocket-subprotocol"
)

// TokenKeyInfo is the parsed form of the incoming API credential together with
// diagnostic metadata. The metadata never contains the raw secret, so it is
// safe to log.
type TokenKeyInfo struct {
	// Parts is the credential split on '-'. Parts[0] is the token; any
	// additional parts are interpreted as an admin channel specification
	// (see TokenAuth). It always has at least one element.
	Parts []string
	// Source records which header/transport supplied the credential.
	Source authTokenSource
	// HadScheme reports whether a `Bearer ` authentication scheme prefix was
	// present and stripped.
	HadScheme bool
}

// extractRawCredential returns the raw credential string from the request and
// the source it was read from. It accepts, in order of precedence:
//
//  1. Authorization header — standard OpenAI `Bearer` scheme.
//  2. X-Api-Key header — Anthropic-compatible.
//  3. Api-Key header — Azure OpenAI-compatible. GitHub Copilot's `azure` BYOK
//     provider type sends the key in this header rather than Authorization.
//  4. Sec-WebSocket-Protocol subprotocol — OpenAI Realtime over WebSocket, for
//     browsers that cannot set custom headers:
//     "Sec-WebSocket-Protocol: realtime, openai-insecure-api-key.{KEY}, openai-beta.realtime-v1"
func extractRawCredential(c *gin.Context) (raw string, source authTokenSource) {
	if v := strings.TrimSpace(c.Request.Header.Get("Authorization")); v != "" {
		return v, authSourceAuthorization
	}
	// compatible with Anthropic
	if v := strings.TrimSpace(c.Request.Header.Get("X-Api-Key")); v != "" {
		return v, authSourceXAPIKey
	}
	// compatible with Azure OpenAI (and GitHub Copilot's `azure` provider type)
	if v := strings.TrimSpace(c.Request.Header.Get("Api-Key")); v != "" {
		return v, authSourceAPIKey
	}

	// For WebSocket upgrade requests, also check subprotocol-based auth.
	// Browsers cannot set custom headers on WebSocket connections, so the
	// OpenAI Realtime API allows passing the key as a subprotocol.
	if sp := c.Request.Header.Get("Sec-WebSocket-Protocol"); sp != "" {
		for _, proto := range strings.Split(sp, ",") {
			proto = strings.TrimSpace(proto)
			if strings.HasPrefix(proto, "openai-insecure-api-key.") {
				return strings.TrimPrefix(proto, "openai-insecure-api-key."), authSourceWebSocket
			}
		}
	}

	return "", authSourceNone
}

// stripAuthScheme removes a leading `Bearer ` authentication scheme from a
// credential value. Per RFC 7235 the auth-scheme token is case-insensitive,
// so `Bearer`, `bearer`, `BEARER`, etc. are all accepted. Any surrounding
// whitespace after the scheme is trimmed so that values such as
// "Bearer  sk-xxx" (extra spaces) do not corrupt later '-' splitting.
func stripAuthScheme(key string) string {
	const scheme = "bearer "
	if len(key) >= len(scheme) && strings.EqualFold(key[:len(scheme)], scheme) {
		return strings.TrimSpace(key[len(scheme):])
	}
	return key
}

// parseTokenKey extracts and normalizes the API credential from the request.
//
// key like `sk-{token}[-{channelid}]`
//
// The returned TokenKeyInfo carries diagnostic metadata (never the raw secret)
// to help diagnose client authentication problems such as non-standard headers.
func parseTokenKey(c *gin.Context) TokenKeyInfo {
	raw, source := extractRawCredential(c)

	key := stripAuthScheme(raw)
	hadScheme := key != raw

	// Trim current configured prefix first
	if p := config.TokenKeyPrefix; p != "" {
		key = strings.TrimPrefix(key, p)
	}
	// Backward compatibility with historical prefixes
	key = strings.TrimPrefix(key, "sk-")
	key = strings.TrimPrefix(key, "laisky-")

	return TokenKeyInfo{
		Parts:     splitTokenKeyParts(key),
		Source:    source,
		HadScheme: hadScheme,
	}
}

// splitTokenKeyParts separates a token key from an optional admin channel suffix.
// Parameters:
//   - key: prefix-stripped credential in the form token, token-intChannel, or token-uuidChannel.
//
// Return values:
//   - []string: one element for a plain token or two elements for token and channel reference.
func splitTokenKeyParts(key string) []string {
	if token, channelRef, ok := splitTokenKeyUUIDSuffix(key); ok {
		return []string{token, channelRef}
	}
	if token, channelRef, ok := splitTokenKeyIntegerSuffix(key); ok {
		return []string{token, channelRef}
	}
	return []string{key}
}

// splitTokenKeyUUIDSuffix extracts a final hyphenated UUID channel suffix from a token key.
// Parameters:
//   - key: prefix-stripped credential to inspect.
//
// Return values:
//   - string: token portion before the UUID suffix.
//   - string: UUID channel reference suffix.
//   - bool: true when a valid final UUID suffix was found.
func splitTokenKeyUUIDSuffix(key string) (string, string, bool) {
	const uuidLen = 36
	if len(key) <= uuidLen+1 {
		return "", "", false
	}
	separator := len(key) - uuidLen - 1
	if key[separator] != '-' {
		return "", "", false
	}
	token := key[:separator]
	channelRef := key[separator+1:]
	if token == "" || !looksHyphenatedUUID(channelRef) {
		return "", "", false
	}
	return token, channelRef, true
}

// splitTokenKeyIntegerSuffix extracts a final decimal integer channel suffix from a token key.
// Parameters:
//   - key: prefix-stripped credential to inspect.
//
// Return values:
//   - string: token portion before the integer suffix.
//   - string: integer channel reference suffix.
//   - bool: true when a valid final integer suffix was found.
func splitTokenKeyIntegerSuffix(key string) (string, string, bool) {
	idx := strings.LastIndex(key, "-")
	if idx <= 0 || idx == len(key)-1 {
		return "", "", false
	}
	token := key[:idx]
	channelRef := key[idx+1:]
	for _, r := range channelRef {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	return token, channelRef, true
}

// looksHyphenatedUUID reports whether ref has canonical hyphenated UUID shape.
// Parameters:
//   - ref: candidate UUID string.
//
// Return values:
//   - bool: true when ref is 36 characters with UUID hyphens and hexadecimal digits.
func looksHyphenatedUUID(ref string) bool {
	if len(ref) != 36 {
		return false
	}
	for i, r := range ref {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isASCIIHexDigit(r) {
				return false
			}
		}
	}
	return true
}

// isASCIIHexDigit reports whether r is an ASCII hexadecimal digit.
// Parameters:
//   - r: rune to inspect.
//
// Return values:
//   - bool: true when r is 0-9, a-f, or A-F.
func isASCIIHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// GetTokenKeyParts extracts the token key parts from the request credential.
//
// key like `sk-{token}[-{channelid}]`
//
// It accepts the standard `Authorization: Bearer` header as well as the
// Anthropic `X-Api-Key` and Azure `Api-Key` headers, and (for WebSocket
// upgrades) the OpenAI Realtime subprotocol form. The `Bearer` scheme match is
// case-insensitive (RFC 7235).
func GetTokenKeyParts(c *gin.Context) []string {
	return parseTokenKey(c).Parts
}
