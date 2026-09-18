package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestJinaURLSecurityPolicy checks strict defaults and an explicit IP-only local
// exception, including redaction of rejected credentials and query values.
func TestJinaURLSecurityPolicy(t *testing.T) {
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			t.Setenv(JinaAllowLoopbackHTTPEnv, enabled)
			for _, raw := range []string{"https://api.jina.ai", "https://proxy.example:8443/v1/rerank?region=ca"} {
				require.NoError(t, ValidateJinaURL(raw))
			}
			for _, raw := range []string{"", "/v1/embeddings", "//api.jina.ai", "https:///path", "https://", "ftp://api.jina.ai", "http://api.jina.ai", "http://10.0.0.1", "http://localhost", "http://localhost.example", "http://127.0.0.1.example", "https://user:secret@example.com/?key=secret", "https://api.jina.ai/#secret", "https://api.jina.ai:70000", "https://api.jina.ai:0"} {
				err := ValidateJinaURL(raw)
				require.Error(t, err, raw)
				require.NotContains(t, err.Error(), "secret")
			}
			for _, raw := range []string{"http://127.0.0.1:8080", "http://[::1]:8080", "http://[::ffff:127.0.0.1]:8080"} {
				err := ValidateJinaURL(raw)
				if enabled == "true" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}
			require.NoError(t, ValidateJinaURLs("", map[string]string{"embeddings": ""}))
			require.Error(t, ValidateJinaURLs("https://api.jina.ai?key=secret", nil))
			require.Error(t, ValidateJinaURLs("https://api.jina.ai", map[string]string{"embeddings": "http://example.com"}))
		})
	}
}
