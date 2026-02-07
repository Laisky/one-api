package network

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateExternalURL ensures private and local hosts are rejected while public IPs pass.
func TestValidateExternalURL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	blocked := []string{
		"http://127.0.0.1/test",
		"http://localhost/test",
		"http://10.0.0.1/test",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/test",
		"http://100.64.0.1/test",
	}

	for _, raw := range blocked {
		_, err := ValidateExternalURL(ctx, raw)
		require.Error(t, err, "expected %s to be blocked", raw)
	}

	allowed := []string{
		"http://8.8.8.8/test",
		"https://1.1.1.1/test",
	}

	for _, raw := range allowed {
		_, err := ValidateExternalURL(ctx, raw)
		require.NoError(t, err, "expected %s to be allowed", raw)
	}

	_, err := ValidateExternalURL(ctx, "ftp://example.com/resource")
	require.Error(t, err)
}
