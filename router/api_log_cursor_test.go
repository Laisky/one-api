package router

// Route registration for the additive keyset log endpoints (proposal
// docs/proposals/20260905_observability-data-tiering.md, W2.4).
//
// This builds the REAL API router rather than a hand-rolled group, because the
// property worth asserting is that the shipped registration is conflict-free
// and that the legacy routes are still exactly where they were.

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestApiRouterRegistersCursorRoutesBesideLegacyOnes verifies the additive
// routes exist, the legacy ones are untouched, and gin accepts the whole tree.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestApiRouterRegistersCursorRoutesBesideLegacyOnes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// gin panics at registration time on a conflicting route, so building the
	// real router IS the conflict assertion.
	require.NotPanics(t, func() { SetApiRouter(engine) })

	registered := make(map[string]bool)
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	for _, path := range []string{
		// Additive.
		"GET /api/log/cursor",
		"GET /api/log/self/cursor",
		// Legacy, which W2.4 must leave exactly as it found them.
		"GET /api/log/",
		"GET /api/log/self",
		"GET /api/log/search",
		"GET /api/log/self/search",
		"GET /api/log/stat",
		"GET /api/log/self/stat",
		"DELETE /api/log/",
	} {
		require.True(t, registered[path], "route %s must be registered", path)
	}
}
