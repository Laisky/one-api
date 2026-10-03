package controller

import (
	"net/http"
	"testing"

	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestAppendRelayFailureFieldsIdentifiesOneAPIUserQuota verifies that local user
// quota failures carry the affected user ID and cannot be mistaken for an upstream
// provider account balance failure.
func TestAppendRelayFailureFieldsIdentifiesOneAPIUserQuota(t *testing.T) {
	t.Parallel()

	fields := appendRelayFailureFields(processChannelRelayErrorParams{
		UserId:  175,
		TokenId: 257,
		Err: model.ErrorWithStatusCode{
			StatusCode: http.StatusForbidden,
			Error: model.Error{
				Message: "user quota is not enough",
				Type:    model.ErrorTypeOneAPI,
				Code:    "insufficient_user_quota",
			},
		},
	})

	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}

	require.Equal(t, int64(175), encoder.Fields["user_id"])
	require.Equal(t, "one_api_user", encoder.Fields["quota_scope"])
	require.Equal(t, "user quota is not enough", encoder.Fields["one_api_error"])
	require.NotContains(t, encoder.Fields, "upstream_error")
	require.Equal(t, int64(257), encoder.Fields["token_id"])
}

// TestAppendRelayFailureFieldsKeepsUpstreamQuotaSeparate verifies that provider
// quota failures retain upstream labeling and do not claim a local one-api scope.
func TestAppendRelayFailureFieldsKeepsUpstreamQuotaSeparate(t *testing.T) {
	t.Parallel()

	fields := appendRelayFailureFields(processChannelRelayErrorParams{
		UserId: 175,
		Err: model.ErrorWithStatusCode{
			StatusCode: http.StatusTooManyRequests,
			Error: model.Error{
				Message: "provider account has insufficient credit",
				Type:    model.ErrorTypeInsufficientQuota,
				Code:    "insufficient_quota",
			},
		},
	})

	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}

	require.Equal(t, "provider account has insufficient credit", encoder.Fields["upstream_error"])
	require.NotContains(t, encoder.Fields, "one_api_error")
	require.NotContains(t, encoder.Fields, "quota_scope")
	require.Equal(t, int64(175), encoder.Fields["user_id"])
}
