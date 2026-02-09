package controller

import (
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

func TestShouldTreatConvertRequestErrorAsBadRequest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Validation failure",
			err:      errors.New("validation failed: model does not support image input"),
			expected: true,
		},
		{
			name:     "Embedding unsupported",
			err:      errors.New("provider does not support embedding"),
			expected: true,
		},
		{
			name:     "Claude endpoint unsupported",
			err:      errors.New("channel does not support the v1/messages endpoint"),
			expected: true,
		},
		{
			name:     "Internal conversion error",
			err:      errors.New("json marshal failed"),
			expected: false,
		},
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, shouldTreatConvertRequestErrorAsBadRequest(tc.err))
		})
	}
}

func TestWrapConvertRequestError(t *testing.T) {
	t.Parallel()

	badRequestErr := wrapConvertRequestError(errors.New("validation failed: invalid multimodal content"))
	require.Equal(t, http.StatusBadRequest, badRequestErr.StatusCode)
	require.Equal(t, "invalid_request_error", badRequestErr.Code)

	internalErr := wrapConvertRequestError(errors.New("marshal converted request failed"))
	require.Equal(t, http.StatusInternalServerError, internalErr.StatusCode)
	require.Equal(t, "convert_request_failed", internalErr.Code)
}
