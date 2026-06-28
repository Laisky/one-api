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
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/relay/model"
)

// AbortWithError aborts the request with an error message
func AbortWithError(c *gin.Context, statusCode int, err error) {
	logger := gmw.GetLogger(c)
	if shouldLogAsWarning(statusCode, err) {
		logger.Warn("server abort",
			zap.Int("status_code", statusCode),
			zap.Error(err))
	} else {
		logger.Error("server abort",
			zap.Int("status_code", statusCode),
			zap.Error(err))
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
// All fields are masked or ID-based to avoid exposing sensitive data.
type TokenInfo struct {
	MaskedKey   string // Masked API key (prefix...suffix)
	TokenId     int    // Token ID
	TokenName   string // Token name
	UserId      int    // User ID who owns the token
	Username    string // Username (optional, may be empty)
	RequestedAt string // Request model (optional)
}

// AbortWithTokenError aborts the request with an error message and logs detailed token information.
// This function should be used when rejecting API key requests to provide more context in logs
// for debugging purposes. The token information is safely masked to avoid exposing sensitive data.
func AbortWithTokenError(c *gin.Context, statusCode int, err error, tokenInfo *TokenInfo) {
	logger := gmw.GetLogger(c)
	logFields := []zap.Field{
		zap.Int("status_code", statusCode),
		zap.Error(err),
	}

	// Add token info fields if available
	if tokenInfo != nil {
		logFields = append(logFields,
			zap.String("api_key", tokenInfo.MaskedKey),
			zap.Int("token_id", tokenInfo.TokenId),
			zap.String("token_name", tokenInfo.TokenName),
			zap.Int("user_id", tokenInfo.UserId),
		)
		if tokenInfo.Username != "" {
			logFields = append(logFields, zap.String("username", tokenInfo.Username))
		}
		if tokenInfo.RequestedAt != "" {
			logFields = append(logFields, zap.String("requested_model", tokenInfo.RequestedAt))
		}
	}

	if shouldLogAsWarning(statusCode, err) {
		logger.Warn("server abort", logFields...)
	} else {
		logger.Error("server abort", logFields...)
	}

	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": helper.MessageWithRequestId(err.Error(), c.GetString(helper.RequestIdKey)),
			"type":    string(model.ErrorTypeOneAPI),
		},
	})
	c.Abort()
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
	if statusCode >= 400 && statusCode < 500 {
		return true
	}

	if err == nil {
		return false
	}

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

func isModelInList(modelName string, models string) bool {
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
		Parts:     strings.Split(key, "-"),
		Source:    source,
		HadScheme: hadScheme,
	}
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
