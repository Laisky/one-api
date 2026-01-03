package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseStatus ensures nil responses are handled without panics and return zero status.
func TestResponseStatus(t *testing.T) {
	t.Parallel()
	t.Run("nil response", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 0, responseStatus(nil))
	})

	t.Run("non-nil response", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{StatusCode: http.StatusTeapot}
		require.Equal(t, http.StatusTeapot, responseStatus(resp))
	})
}
