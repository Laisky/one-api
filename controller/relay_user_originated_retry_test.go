package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestRelayRetryBudget_UserQuotaError reproduces the regression from the
// 2026-08-14 trace: a user-originated 403 (insufficient_user_quota) used to pass
// the generic retry policy (401/403 are retryable), enter the relay retry loop,
// and log a misleading ERROR "relay retry channel selection failed" /
// "no available channels support model X after exclusions" when no alternative
// channel existed. Switching channels cannot fix a caller-side quota error, so
// the retry budget must be zero.
func TestRelayRetryBudget_UserQuotaError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	bizErr := &model.ErrorWithStatusCode{
		StatusCode: http.StatusForbidden,
		Error: model.Error{
			Type:    model.ErrorTypeOneAPI,
			Code:    "insufficient_user_quota",
			Message: "user quota is not enough",
		},
	}

	// Mirror the retry-budget computation performed by controller.Relay
	// (relay.go: retryTimes := config.RetryTimes; ...; shouldRetry(...)).
	const baseRetryTimes = 1 // production deployments set config.RetryTimes > 0
	retryableClientError, _ := classifyRetryableUpstreamClientError(bizErr)
	retryTimes := baseRetryTimes
	if err := shouldRetry(c, bizErr); err != nil {
		if !retryableClientError {
			retryTimes = 0
		}
	}

	require.Zero(t, retryTimes,
		"user-originated quota errors must never enter the relay retry loop")
}

// TestRelayRetryBudget_UpstreamAuthErrorStillRetries guards backward
// compatibility: an upstream 403 (not a one-api user-originated error) may still
// be fixed by another channel, so it must keep the relay retry budget.
func TestRelayRetryBudget_UpstreamAuthErrorStillRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	bizErr := &model.ErrorWithStatusCode{
		StatusCode: http.StatusForbidden,
		Error: model.Error{
			Type:    model.ErrorTypePermission,
			Code:    "permission_denied",
			Message: "upstream denied the request",
		},
	}

	const baseRetryTimes = 1
	retryableClientError, _ := classifyRetryableUpstreamClientError(bizErr)
	retryTimes := baseRetryTimes
	if err := shouldRetry(c, bizErr); err != nil {
		if !retryableClientError {
			retryTimes = 0
		}
	}

	require.Equal(t, baseRetryTimes, retryTimes,
		"upstream 403 errors must keep the relay retry budget")
}

// TestShouldRetry_UserOriginatedMatrix locks in the retry-policy regression suite:
// one-api user-originated 401/403 failures must never be retried, while upstream
// auth/permission failures and server/rate-limit errors keep their retry budget.
func TestShouldRetry_UserOriginatedMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userOriginated := []struct {
		name string
		err  *model.ErrorWithStatusCode
	}{
		{
			name: "insufficient user quota",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusForbidden,
				Error:      model.Error{Type: model.ErrorTypeOneAPI, Code: "insufficient_user_quota", Message: "user quota is not enough"},
			},
		},
		{
			name: "insufficient token quota",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusForbidden,
				Error:      model.Error{Type: model.ErrorTypeOneAPI, Code: "insufficient_token_quota", Message: "token quota is not enough"},
			},
		},
		{
			name: "pre-consume token quota failed",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusForbidden,
				Error:      model.Error{Type: model.ErrorTypeOneAPI, Code: "pre_consume_token_quota_failed", Message: "insufficient token quota: required=100, available=0, tokenId=1"},
			},
		},
		{
			name: "invalid api key",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusUnauthorized,
				Error:      model.Error{Type: model.ErrorTypeOneAPI, Code: "invalid_api_key", Message: "api key is invalid"},
			},
		},
		{
			name: "token expired",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusUnauthorized,
				Error:      model.Error{Type: model.ErrorTypeOneAPI, Code: "token_expired", Message: "token has expired"},
			},
		},
		{
			name: "model not allowed",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusForbidden,
				Error:      model.Error{Type: model.ErrorTypeOneAPI, Code: "model_not_allowed", Message: "model not allowed by token whitelist"},
			},
		},
	}

	retryable := []struct {
		name string
		err  *model.ErrorWithStatusCode
	}{
		{
			name: "upstream permission denied",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusForbidden,
				Error:      model.Error{Type: model.ErrorTypePermission, Code: "permission_denied", Message: "upstream denied the request"},
			},
		},
		{
			name: "upstream authentication failure",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusUnauthorized,
				Error:      model.Error{Type: model.ErrorTypeAuthentication, Code: "invalid_api_key", Message: "invalid api key"},
			},
		},
		{
			name: "generic forbidden without one-api classification",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusForbidden,
				Error:      model.Error{Message: "forbidden"},
			},
		},
		{
			name: "server error",
			err:  &model.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError},
		},
		{
			name: "rate limited",
			err:  &model.ErrorWithStatusCode{StatusCode: http.StatusTooManyRequests},
		},
		{
			name: "payload too large",
			err:  &model.ErrorWithStatusCode{StatusCode: http.StatusRequestEntityTooLarge},
		},
	}

	for _, tc := range userOriginated {
		t.Run("skip_"+tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			require.Error(t, shouldRetry(c, tc.err),
				"user-originated relay error %q must not be retried", tc.err.Code)
		})
	}

	for _, tc := range retryable {
		t.Run("retry_"+tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			require.NoError(t, shouldRetry(c, tc.err),
				"relay error %q must keep its retry budget", tc.err.Code)
		})
	}
}

// TestRelayErrorCodeOrMessage guards the diagnostic label used in the retry-skip
// reason so the trace's insufficient_user_quota case stays identifiable.
func TestRelayErrorCodeOrMessage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "insufficient_user_quota", relayErrorCodeOrMessage(&model.ErrorWithStatusCode{
		Error: model.Error{Code: "insufficient_user_quota", Message: "user quota is not enough"},
	}))
	require.Equal(t, "user quota is not enough", relayErrorCodeOrMessage(&model.ErrorWithStatusCode{
		Error: model.Error{Message: "user quota is not enough"},
	}))
	require.Equal(t, "unknown", relayErrorCodeOrMessage(nil))
	require.Equal(t, "unknown", relayErrorCodeOrMessage(&model.ErrorWithStatusCode{}))
}
