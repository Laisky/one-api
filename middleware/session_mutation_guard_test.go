package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestSessionMutationProvenance exercises Fetch Metadata precedence, exact
// trusted origins and legacy fallbacks, including TLS termination, Host
// rewriting proxies and separately hosted frontends.
func TestSessionMutationProvenance(t *testing.T) {
	oldServer, oldFrontend, oldSecure := config.ServerAddress, config.FrontendBaseURL, config.EnableCookieSecure
	config.ServerAddress, config.FrontendBaseURL, config.EnableCookieSecure = "https://gateway.test", "https://dashboard.test/app", true
	t.Cleanup(func() {
		config.ServerAddress, config.FrontendBaseURL, config.EnableCookieSecure = oldServer, oldFrontend, oldSecure
	})
	for _, tc := range []struct {
		name, origin, referer, site string
		want                        bool
	}{
		{"same_origin", "https://gateway.test", "", "same-origin", true},
		{"default_port", "https://GATEWAY.test:443", "", "same-origin", true},
		{"explicit_frontend", "https://dashboard.test", "", "cross-site", true},
		{"metadata_fallback", "", "", "same-origin", true},
		{"referer_fallback", "", "https://gateway.test/settings?tab=account", "", true},
		{"absent", "", "", "", false},
		{"none_is_not_proof", "", "", "none", false},
		{"cross_site", "https://evil.test", "", "cross-site", false},
		{"sibling_site", "https://sibling.gateway.test", "", "same-site", false},
		{"metadata_beats_rewritten_host", "https://public.example.test", "", "same-origin", true},
		{"metadata_beats_null_origin", "null", "", "same-origin", true},
		{"trusted_origin_without_metadata", "https://gateway.test", "", "", true},
		{"wrong_port", "https://gateway.test:444", "", "", false},
		{"wrong_port_cross_site", "https://gateway.test:444", "", "cross-site", false},
		{"wrong_scheme", "http://gateway.test", "", "", false},
		{"null_origin", "null", "https://gateway.test/", "", false},
		{"null_origin_cross_site", "null", "https://gateway.test/", "cross-site", false},
		{"origin_path", "https://gateway.test/path", "", "", false},
		{"origin_credentials", "https://user@gateway.test", "", "", false},
		{"origin_query", "https://gateway.test?", "", "", false},
		{"origin_fragment", "https://gateway.test/#x", "", "", false},
		{"multiple_origins", "https://gateway.test https://evil.test", "", "", false},
		{"evil_referer", "", "https://evil.test/settings", "", false},
		{"contradictory_referer", "", "https://gateway.test/settings", "cross-site", false},
		{"same_site_referer", "", "https://gateway.test/settings", "same-site", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://gateway.test/api/user/token", nil)
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Referer", tc.referer)
			request.Header.Set("Sec-Fetch-Site", tc.site)
			require.Equal(t, tc.want, trustedSessionMutation(request))
		})
	}
	request := httptest.NewRequest(http.MethodPost, "http://gateway.test/api/user/token", nil)
	request.Header.Set("Origin", "https://gateway.test")
	request.Header.Add("Origin", "https://evil.test")
	require.False(t, trustedSessionMutation(request))
	request.Header.Set("Origin", "https://gateway.test")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Add("Sec-Fetch-Site", "cross-site")
	require.False(t, trustedSessionMutation(request), "duplicate Fetch Metadata must fail closed")
	request.Header.Del("Sec-Fetch-Site")
	request.Header.Set("Origin", "https://evil.test")
	request.Header.Set("X-Forwarded-Host", "evil.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	require.False(t, trustedSessionMutation(request))
}

// TestSessionMutationConcurrentServerAddress verifies origin checks can run while
// an administrator updates the public URL using the option writer's lock.
func TestSessionMutationConcurrentServerAddress(t *testing.T) {
	config.OptionMapRWMutex.Lock()
	previous := config.ServerAddress
	config.ServerAddress = "https://first.test"
	config.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		config.OptionMapRWMutex.Lock()
		config.ServerAddress = previous
		config.OptionMapRWMutex.Unlock()
	})
	var workers sync.WaitGroup
	workers.Go(func() {
		for range 1000 {
			config.OptionMapRWMutex.Lock()
			config.ServerAddress = "https://second.test"
			config.ServerAddress = "https://first.test"
			config.OptionMapRWMutex.Unlock()
		}
	})
	request := httptest.NewRequest(http.MethodPost, "https://gateway.test/api/user/token", nil)
	request.Header.Set("Origin", "https://attacker.test")
	for range 1000 {
		require.False(t, trustedSessionMutation(request))
	}
	workers.Wait()
}
