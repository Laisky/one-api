package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestSetRelayRouterDoesNotExposeRealtimeSessions verifies that relay route
// registration does not expose unmetered OpenAI ephemeral credential minting.
func TestSetRelayRouterDoesNotExposeRealtimeSessions(t *testing.T) {
	t.Parallel()

	router := gin.New()
	SetRelayRouter(router)

	for _, route := range router.Routes() {
		require.NotEqual(t, "/v1/realtime/sessions", route.Path)
	}
}
