package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// TestShouldRetryContextCancellation ensures we do not retry when rawErr indicates
// a context cancellation even if status code is 500.
func TestShouldRetryContextCancellation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, &model.ErrorWithStatusCode{
		StatusCode: http.StatusInternalServerError,
		Error:      model.Error{RawError: context.Canceled},
	})
	assert.Error(t, err, "should not retry when context is canceled causing 5xx")
}

// TestShouldRetryDeadlineExceeded ensures we do not retry when rawErr indicates
// a deadline exceeded even if status code is 500.
func TestShouldRetryDeadlineExceeded(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, &model.ErrorWithStatusCode{
		StatusCode: http.StatusInternalServerError,
		Error:      model.Error{RawError: context.DeadlineExceeded},
	})
	assert.Error(t, err, "should not retry when context deadline exceeded causing 5xx")
}

// TestShouldRetryNormal500 ensures we DO retry for a normal 500 without context cancellation.
func TestShouldRetryNormal500(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, &model.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError})
	assert.NoError(t, err, "should retry for normal 500 error")
}

// TestShouldRetryClientError ensures client 400 errors still do not retry (except whitelisted codes).
func TestShouldRetryClientError(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	err := shouldRetry(c, &model.ErrorWithStatusCode{StatusCode: http.StatusBadRequest})
	assert.Error(t, err, "should not retry for 400 bad request")
}

// TestShouldRetry verifies that shouldRetry allows a retry for retryable
// statuses and refuses one when the caller pinned a specific channel.
func TestShouldRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		statusCode      int
		specificChannel int
		expectError     bool
		errorContains   string
	}{
		{
			name:            "should retry on 200 OK",
			statusCode:      http.StatusOK,
			specificChannel: 0,
			expectError:     false,
		},
		{
			name:            "should retry on 429 when no specific channel",
			statusCode:      http.StatusTooManyRequests,
			specificChannel: 0,
			expectError:     false,
		},
		{
			name:            "should not retry when specific channel ID is set",
			statusCode:      http.StatusTooManyRequests,
			specificChannel: 123,
			expectError:     true,
			errorContains:   "specific channel ID (123) was provided",
		},
		{
			name:            "should retry on 500 error",
			statusCode:      http.StatusInternalServerError,
			specificChannel: 0,
			expectError:     false,
		},
		{
			name:            "should retry on 503 error",
			statusCode:      http.StatusServiceUnavailable,
			specificChannel: 0,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			c.Set(ctxkey.SpecificChannelId, tt.specificChannel)

			err := shouldRetry(c, &model.ErrorWithStatusCode{StatusCode: tt.statusCode})

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestShouldRetryLogic verifies the retry gate controller.Relay applies after a
// failed attempt: shouldRetry decides whether another channel may be tried, a
// known-transient upstream 4xx shape (classifyRetryableUpstreamClientError)
// keeps the retry budget, and a pinned specific channel always vetoes a retry.
// It asserts the decision itself rather than a wall-clock bound: the former
// bound timed only test bookkeeping and failed under scheduler load.
func TestShouldRetryLogic(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		statusCode      int
		code            string
		specificChannel int
		wantRetry       bool
		wantVeto        string
	}{
		{
			name:            "400 with specific channel should not retry",
			statusCode:      http.StatusBadRequest,
			specificChannel: 42,
			wantVeto:        "specific channel ID (42)",
		},
		{
			name:       "400 without specific channel should not retry channels",
			statusCode: http.StatusBadRequest,
			wantVeto:   "client error 400",
		},
		{
			name:       "transient upstream 400 keeps the retry budget",
			statusCode: http.StatusBadRequest,
			code:       "output_parse_failed",
			wantRetry:  true,
			wantVeto:   "client error 400",
		},
		{
			name:            "429 with specific channel should not retry",
			statusCode:      http.StatusTooManyRequests,
			specificChannel: 42,
			wantVeto:        "specific channel ID (42)",
		},
		{
			name:       "429 should retry with other channels",
			statusCode: http.StatusTooManyRequests,
			wantRetry:  true,
		},
		{
			name:       "500 should retry with other channels",
			statusCode: http.StatusInternalServerError,
			wantRetry:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.SpecificChannelId, tt.specificChannel)
			bizErr := &model.ErrorWithStatusCode{
				StatusCode: tt.statusCode,
				Error: model.Error{
					Message: "upstream failure",
					Type:    model.ErrorTypeUpstream,
					Code:    tt.code,
				},
			}

			// Mirror the gate in controller.Relay: a shouldRetry veto stops the
			// retry loop unless the failure is a known-transient upstream 4xx.
			vetoErr := shouldRetry(c, bizErr)
			retryable, _ := classifyRetryableUpstreamClientError(bizErr)
			if tt.wantVeto == "" {
				require.NoError(t, vetoErr)
			} else {
				require.ErrorContains(t, vetoErr, tt.wantVeto)
			}
			retryAllowed := vetoErr == nil || (retryable && !videoReplayUnsafe(c))
			require.Equal(t, tt.wantRetry, retryAllowed)
			if tt.wantRetry {
				require.Equal(t, retrySelectStrictPriority, retrySelectionPolicyFor(tt.statusCode),
					"a retry must walk the remaining channels strictly by priority")
			}
		})
	}
}
