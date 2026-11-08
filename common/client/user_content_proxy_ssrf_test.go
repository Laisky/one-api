package client

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// recordingForwardProxy is a fake forward proxy that records every request it
// is asked to relay and answers each with an empty image, so a test can tell
// which targets the user-content client actually sent out.
type recordingForwardProxy struct {
	mu      sync.Mutex
	targets []string
}

// ServeHTTP records the relayed target (absolute URL, or host for CONNECT)
// and replies with an empty PNG.
func (p *recordingForwardProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.URL.String()
	if r.Method == http.MethodConnect {
		target = "CONNECT " + r.Host
	}
	p.mu.Lock()
	p.targets = append(p.targets, target)
	p.mu.Unlock()
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
}

// seen returns a copy of the recorded targets.
func (p *recordingForwardProxy) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.targets...)
}

// userContentClientThroughProxy initializes the real user-content client with
// USER_CONTENT_REQUEST_PROXY pointing at a recording proxy and restores the
// shared client state afterwards.
func userContentClientThroughProxy(t *testing.T) (*http.Client, *recordingForwardProxy) {
	t.Helper()
	recorder := &recordingForwardProxy{}
	proxy := httptest.NewServer(recorder)
	oldProxy, oldRelayProxy := config.UserContentRequestProxy, config.RelayProxy
	oldClient, oldImpatient, oldContent := HTTPClient, ImpatientHTTPClient, UserContentRequestHTTPClient
	config.UserContentRequestProxy, config.RelayProxy = proxy.URL, ""
	Init()
	t.Cleanup(func() {
		UserContentRequestHTTPClient.CloseIdleConnections()
		proxy.Close()
		config.UserContentRequestProxy, config.RelayProxy = oldProxy, oldRelayProxy
		HTTPClient, ImpatientHTTPClient, UserContentRequestHTTPClient = oldClient, oldImpatient, oldContent
	})
	return UserContentRequestHTTPClient, recorder
}

// TestUserContentClientProxyRefusesPrivateTargets proves that with a
// user-content proxy configured, the shared client itself refuses private,
// loopback and metadata targets before contacting the proxy. Behind a proxy the
// dial guard only sees the proxy address, so callers that use the client
// directly (as the Ali image download and AWS image paths do) previously had
// their private targets relayed by the proxy.
func TestUserContentClientProxyRefusesPrivateTargets(t *testing.T) {
	httpClient, recorder := userContentClientThroughProxy(t)

	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:8080/admin",
		"http://10.0.0.5/internal.png",
		"https://10.0.0.5/internal.png",
		"http://[64:ff9b::a9fe:a9fe]/latest/meta-data/",
		"http://localhost/secret.png",
	} {
		resp, err := httpClient.Get(target)
		if resp != nil {
			require.NoError(t, resp.Body.Close())
		}
		require.Error(t, err, "the user-content client must refuse %s", target)
	}
	require.Empty(t, recorder.seen(), "no private target may reach the proxy")

	// Control: a public target is still relayed, proving the proxy path works.
	resp, err := httpClient.Get("http://8.8.8.8/image.png")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"http://8.8.8.8/image.png"}, recorder.seen())
}
