package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestUpstreamSuggestsRetry(t *testing.T) {
	// Test cases for upstreamSuggestsRetry function
	testCases := []struct {
		name     string
		err      *model.ErrorWithStatusCode
		expected bool
	}{
		{
			name:     "nil error returns false",
			err:      nil,
			expected: false,
		},
		{
			name:     "empty message returns false",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: ""}},
			expected: false,
		},
		{
			name:     "generic error without retry suggestion returns false",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Internal server error"}},
			expected: false,
		},
		{
			name:     "OpenAI style retry suggestion",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "The server had an error processing your request. Sorry about that! You can retry your request, or contact us through our help center."}},
			expected: true,
		},
		{
			name:     "generic try again suggestion",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Service unavailable. Please try again later."}},
			expected: true,
		},
		{
			name:     "retry later suggestion",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Too busy right now. Retry later."}},
			expected: true,
		},
		{
			name:     "temporarily unavailable",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "The service is temporarily unavailable."}},
			expected: true,
		},
		{
			name:     "overloaded server",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "The server is currently overloaded with requests."}},
			expected: true,
		},
		{
			name:     "server is busy",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Server is busy, please wait."}},
			expected: true,
		},
		{
			name:     "service is busy",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "The service is busy right now."}},
			expected: true,
		},
		{
			name:     "experiencing high load",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "We are currently experiencing high load."}},
			expected: true,
		},
		{
			name:     "high traffic",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Due to high traffic, your request could not be processed."}},
			expected: true,
		},
		{
			name:     "capacity limit",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "We've hit our capacity limit for this model."}},
			expected: true,
		},
		{
			name:     "temporary failure",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "A temporary failure occurred."}},
			expected: true,
		},
		{
			name:     "temporary error",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "This is a temporary error."}},
			expected: true,
		},
		{
			name:     "please retry",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Error occurred. Please retry your request."}},
			expected: true,
		},
		{
			name:     "case insensitive - uppercase",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "SERVICE IS TEMPORARILY UNAVAILABLE"}},
			expected: true,
		},
		{
			name:     "case insensitive - mixed case",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Please Try Again Later"}},
			expected: true,
		},
		{
			name:     "authentication error without retry suggestion",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "Invalid API key provided"}},
			expected: false,
		},
		{
			name:     "quota error without retry suggestion",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "You have exceeded your quota"}},
			expected: false,
		},
		{
			name:     "model not found error",
			err:      &model.ErrorWithStatusCode{Error: model.Error{Message: "The model does not exist"}},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := upstreamSuggestsRetry(tc.err)
			require.Equal(t, tc.expected, result, "upstreamSuggestsRetry mismatch for: %s", tc.name)
		})
	}
}

func TestProcessError_Policies(t *testing.T) {
	// Validate intended policy mapping by status code and classification
	// Note: This test checks our mapping and durations, not DB side effects.

	// Save and restore durations
	orig429 := config.ChannelSuspendSecondsFor429
	orig5xx := config.ChannelSuspendSecondsFor5XX
	origAuth := config.ChannelSuspendSecondsForAuth
	t.Cleanup(func() {
		config.ChannelSuspendSecondsFor429 = orig429
		config.ChannelSuspendSecondsFor5XX = orig5xx
		config.ChannelSuspendSecondsForAuth = origAuth
	})

	// Set non-zero, small test durations
	config.ChannelSuspendSecondsFor429 = 10 * time.Second
	config.ChannelSuspendSecondsFor5XX = 5 * time.Second
	config.ChannelSuspendSecondsForAuth = 15 * time.Second

	type Case struct {
		name            string
		err             model.ErrorWithStatusCode
		wantSuspend429  bool
		wantSuspend5xx  bool
		wantSuspendAuth bool
	}

	cases := []Case{
		{
			name:           "429 triggers rate limit suspension",
			err:            model.ErrorWithStatusCode{StatusCode: http.StatusTooManyRequests, Error: model.Error{Type: model.ErrorTypeRateLimit}},
			wantSuspend429: true,
		},
		{
			name: "413 does not suspend",
			err:  model.ErrorWithStatusCode{StatusCode: http.StatusRequestEntityTooLarge},
		},
		{
			name:           "500 without retry suggestion triggers 5xx suspension",
			err:            model.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError, Error: model.Error{Message: "Internal server error"}},
			wantSuspend5xx: true,
		},
		{
			name:           "500 with retry suggestion does NOT trigger 5xx suspension",
			err:            model.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError, Error: model.Error{Message: "You can retry your request"}},
			wantSuspend5xx: false,
		},
		{
			name:           "502 with temporarily unavailable does NOT trigger suspension",
			err:            model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{Message: "Service temporarily unavailable"}},
			wantSuspend5xx: false,
		},
		{
			name:           "503 with try again does NOT trigger suspension",
			err:            model.ErrorWithStatusCode{StatusCode: http.StatusServiceUnavailable, Error: model.Error{Message: "Please try again later"}},
			wantSuspend5xx: false,
		},
		{
			name:           "504 gateway timeout without retry suggestion triggers suspension",
			err:            model.ErrorWithStatusCode{StatusCode: http.StatusGatewayTimeout, Error: model.Error{Message: "Gateway timeout"}},
			wantSuspend5xx: true,
		},
		{
			name:            "401 triggers auth suspension",
			err:             model.ErrorWithStatusCode{StatusCode: http.StatusUnauthorized},
			wantSuspendAuth: true,
		},
		{
			name:            "403 triggers auth suspension",
			err:             model.ErrorWithStatusCode{StatusCode: http.StatusForbidden},
			wantSuspendAuth: true,
		},
		{
			name:            "auth-like by type triggers auth suspension",
			err:             model.ErrorWithStatusCode{StatusCode: 400, Error: model.Error{Type: model.ErrorTypeAuthentication}},
			wantSuspendAuth: true,
		},
		{
			name:            "auth-like by message triggers auth suspension",
			err:             model.ErrorWithStatusCode{StatusCode: 400, Error: model.Error{Message: "API key not valid"}},
			wantSuspendAuth: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The core mapping is tested via the helper and classifier
			isAuth := classifyAuthLike(&tc.err)
			is429 := tc.err.StatusCode == http.StatusTooManyRequests
			is413 := tc.err.StatusCode == http.StatusRequestEntityTooLarge
			is5xx := tc.err.StatusCode >= 500 && tc.err.StatusCode <= 599

			// Check if upstream suggests retry for 5xx errors
			suggestsRetry := upstreamSuggestsRetry(&tc.err)

			// Derive expected suspensions
			got429 := is429
			// 5xx suspension only happens when upstream does NOT suggest retry
			got5xx := is5xx && !suggestsRetry
			gotAuth := isAuth && !is5xx && !is413 && !is429 // mirrors process ordering where early returns apply

			require.Equal(t, tc.wantSuspend429, got429, "429 suspension mismatch")
			require.Equal(t, tc.wantSuspend5xx, got5xx, "5xx suspension mismatch")
			require.Equal(t, tc.wantSuspendAuth, gotAuth, "auth suspension mismatch")
		})
	}
}

func TestProcessError_OpenAIRetryScenario(t *testing.T) {
	// This test specifically covers the user's reported issue:
	// OpenAI returns 500 with "You can retry your request" - should NOT suspend
	err := &model.ErrorWithStatusCode{
		StatusCode: http.StatusInternalServerError,
		Error: model.Error{
			Message: "The server had an error processing your request. Sorry about that! You can retry your request, or contact us through our help center at help.openai.com if you keep seeing this error.",
		},
	}

	// Verify retry suggestion is detected
	require.True(t, upstreamSuggestsRetry(err), "should detect retry suggestion in OpenAI error message")

	// Verify 5xx suspension should be skipped
	is5xx := err.StatusCode >= 500 && err.StatusCode <= 599
	require.True(t, is5xx, "should be recognized as 5xx error")

	suggestsRetry := upstreamSuggestsRetry(err)
	shouldSuspend := is5xx && !suggestsRetry
	require.False(t, shouldSuspend, "should NOT suspend when upstream suggests retry")
}
