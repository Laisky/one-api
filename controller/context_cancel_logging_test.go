package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test classification helper directly
func TestIsClientContextCancel(t *testing.T) {
	t.Parallel()
	require.True(t, isClientContextCancel(http.StatusInternalServerError, context.Canceled),
		"expected true for context.Canceled")
	require.True(t, isClientContextCancel(http.StatusInternalServerError, context.DeadlineExceeded),
		"expected true for context.DeadlineExceeded")
	require.True(t, isClientContextCancel(http.StatusRequestTimeout, nil),
		"expected true for 408 even if rawErr is nil")
	require.False(t, isClientContextCancel(http.StatusInternalServerError, nil),
		"expected false for 500 with nil rawErr")
}
