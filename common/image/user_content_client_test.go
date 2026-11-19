package image

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
)

// TestUserContentRequestWithoutInitializedClient proves that fetching user
// content before client.Init (tests, tools, or a misordered bootstrap) fails
// with an error instead of panicking on the nil SSRF-guarded client.
func TestUserContentRequestWithoutInitializedClient(t *testing.T) {
	previous := client.UserContentRequestHTTPClient
	client.UserContentRequestHTTPClient = nil
	t.Cleanup(func() { client.UserContentRequestHTTPClient = previous })

	require.NotPanics(t, func() {
		resp, err := userContentRequest(context.Background(), "HEAD", "https://93.184.216.34/image.png")
		require.Error(t, err)
		require.Nil(t, resp)
		require.Contains(t, err.Error(), "not initialized")
	})
}
