package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestShouldRetry_ClientAndAuthMatrix(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		status      int
		expectRetry bool
	}{
		{name: "400 bad request should not retry", status: http.StatusBadRequest, expectRetry: false},
		{name: "404 not found should not retry", status: http.StatusNotFound, expectRetry: false},
		{name: "413 capacity should retry", status: http.StatusRequestEntityTooLarge, expectRetry: true},
		{name: "429 rate limit should retry", status: http.StatusTooManyRequests, expectRetry: true},
		{name: "401 unauthorized should retry", status: http.StatusUnauthorized, expectRetry: true},
		{name: "403 forbidden should retry", status: http.StatusForbidden, expectRetry: true},
		{name: "500 server should retry", status: http.StatusInternalServerError, expectRetry: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(nil)
			c.Set(ctxkey.SpecificChannelId, 0)
			err := shouldRetry(c, tc.status, nil)
			if tc.expectRetry {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}

	// When specific channel is pinned, never retry regardless of status
	c, _ := gin.CreateTestContext(nil)
	c.Set(ctxkey.SpecificChannelId, 42)
	assert.Error(t, shouldRetry(c, http.StatusTooManyRequests, nil))
}

func TestClassifyAuthLike(t *testing.T) {
	t.Parallel()
	t.Run("nil error", func(t *testing.T) {
		t.Parallel()
		assert.False(t, classifyAuthLike(nil))
	})

	t.Run("401/403 direct", func(t *testing.T) {
		t.Parallel()
		e1 := &model.ErrorWithStatusCode{StatusCode: http.StatusUnauthorized}
		e2 := &model.ErrorWithStatusCode{StatusCode: http.StatusForbidden}
		assert.True(t, classifyAuthLike(e1))
		assert.True(t, classifyAuthLike(e2))
	})

	t.Run("type-based", func(t *testing.T) {
		t.Parallel()
		for _, typ := range []model.ErrorType{
			model.ErrorTypeAuthentication,
			model.ErrorTypePermission,
			model.ErrorTypeInsufficientQuota,
			model.ErrorTypeForbidden,
		} {
			e := &model.ErrorWithStatusCode{Error: model.Error{Type: typ}}
			assert.True(t, classifyAuthLike(e), typ)
		}
	})

	t.Run("code-based", func(t *testing.T) {
		t.Parallel()
		for _, code := range []any{"invalid_api_key", "account_deactivated", "insufficient_quota"} {
			e := &model.ErrorWithStatusCode{Error: model.Error{Code: code}}
			assert.True(t, classifyAuthLike(e), code)
		}
	})

	t.Run("message-based", func(t *testing.T) {
		t.Parallel()
		msgs := []string{
			"API key not valid",
			"API KEY EXPIRED",
			"insufficient quota for this org",
			"已欠费，余额不足",
			"organization restricted",
		}
		for _, m := range msgs {
			e := &model.ErrorWithStatusCode{Error: model.Error{Message: m}}
			assert.True(t, classifyAuthLike(e), m)
		}
	})

	t.Run("non-auth server error", func(t *testing.T) {
		t.Parallel()
		e := &model.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError, Error: model.Error{Message: "internal error"}}
		assert.False(t, classifyAuthLike(e))
	})
}

func TestIsUserOriginatedRelayError(t *testing.T) {
	t.Parallel()

	t.Run("nil error", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isUserOriginatedRelayError(nil))
	})

	t.Run("oneapi bad request", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: model.Error{
				Type: model.ErrorTypeOneAPI,
				Code: "invalid_text_request",
			},
		}
		assert.True(t, isUserOriginatedRelayError(err))
	})

	t.Run("insufficient user quota", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Type:    model.ErrorTypeOneAPI,
				Code:    "insufficient_user_quota",
				Message: "user quota is not enough",
			},
		}
		assert.True(t, isUserOriginatedRelayError(err))
	})

	t.Run("insufficient token quota from pre consume", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Type:    model.ErrorTypeOneAPI,
				Code:    "pre_consume_token_quota_failed",
				Message: "insufficient token quota: required=100, available=0, tokenId=1",
			},
		}
		assert.True(t, isUserOriginatedRelayError(err))
	})

	t.Run("non-user pre consume failure", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Type:    model.ErrorTypeOneAPI,
				Code:    "pre_consume_token_quota_failed",
				Message: "failed to get token for pre-consume: tokenId=1",
			},
		}
		assert.False(t, isUserOriginatedRelayError(err))
	})

	t.Run("upstream auth failure should not be treated as user-originated", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Type:    model.ErrorTypeAuthentication,
				Code:    "invalid_api_key",
				Message: "invalid api key",
			},
		}
		assert.False(t, isUserOriginatedRelayError(err))
	})

	t.Run("oneapi token expired should be user-originated", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusUnauthorized,
			Error: model.Error{
				Type:    model.ErrorTypeOneAPI,
				Code:    "token_expired",
				Message: "token has expired",
			},
		}
		assert.True(t, isUserOriginatedRelayError(err))
	})

	t.Run("oneapi model whitelist restriction should be user-originated", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Type:    model.ErrorTypeOneAPI,
				Code:    "model_not_allowed",
				Message: "model not allowed by token whitelist",
			},
		}
		assert.True(t, isUserOriginatedRelayError(err))
	})

	t.Run("oneapi token model permission message should be user-originated", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Type:    model.ErrorTypeOneAPI,
				Code:    "forbidden",
				Message: "model not allowed for this token",
			},
		}
		assert.True(t, isUserOriginatedRelayError(err))
	})

	t.Run("oneapi token balance exhausted message should be user-originated", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Type:    model.ErrorTypeOneAPI,
				Code:    "forbidden",
				Message: "API key quota has been exhausted",
			},
		}
		assert.True(t, isUserOriginatedRelayError(err))
	})
}

func TestIsRetryableUpstreamClientError(t *testing.T) {
	t.Parallel()

	t.Run("websocket connection limit code is retryable", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: model.Error{
				Code:    "websocket_connection_limit_reached",
				Message: "Responses websocket connection limit reached (60 minutes).",
			},
		}
		assert.True(t, isRetryableUpstreamClientError(err))
	})

	t.Run("websocket reconnection message is retryable", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: model.Error{
				Message: "Create a new websocket connection to continue.",
			},
		}
		assert.True(t, isRetryableUpstreamClientError(err))
	})

	t.Run("normal bad request is not retryable", func(t *testing.T) {
		t.Parallel()
		err := &model.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: model.Error{
				Code:    "invalid_request_error",
				Message: "tool schema invalid",
			},
		}
		assert.False(t, isRetryableUpstreamClientError(err))
	})
}
