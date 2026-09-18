package jina

import (
	"io"
	"net/http"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// jinaSecurityTransport records dispatched URLs and credentials without network
// access. It optionally emits one redirect to exercise net/http's real policy.
type jinaSecurityTransport struct {
	requests []*http.Request
	status   int
	location string
}

// RoundTrip records req and returns a redirect once or an empty JSON response.
func (tr *jinaSecurityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.requests = append(tr.requests, req.Clone(req.Context()))
	status := http.StatusOK
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	if tr.status != 0 && len(tr.requests) == 1 {
		status = tr.status
		headers.Set("Location", tr.location)
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
}

// configureJinaSecurityTracing makes these synthetic transport-only contexts
// explicitly untraced. They do not install database or tracing middleware
// fixtures; production tracing behavior is tested in its own package.
// These tests are serial, and cleanup restores the global before parallel tests.
func configureJinaSecurityTracing(t *testing.T) {
	t.Helper()
	previous := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = previous })
}

// TestJinaDispatchValidatesEffectiveURL verifies unsafe base and endpoint URLs
// never dispatch, and final header setup rejects an unsafe override before
// copying credentials. Local HTTP requires explicit, literal-loopback opt-in.
func TestJinaDispatchValidatesEffectiveURL(t *testing.T) {
	configureJinaSecurityTracing(t)
	for _, tc := range []struct {
		name, base, override, optIn string
		allowed                     bool
	}{
		{"default", "", "", "false", true},
		{"https_proxy", "https://proxy.example/v1", "", "false", true},
		{"http_base", "http://proxy.example", "", "false", false},
		{"http_override", "https://api.jina.ai", "http://proxy.example/rank", "false", false},
		{"https_override", "https://api.jina.ai", "https://proxy.example/embed", "false", true},
		{"loopback_disabled", "http://127.0.0.1:8080", "", "false", false},
		{"loopback_enabled", "http://127.0.0.1:8080", "", "true", true},
		{"private_http_never", "http://10.0.0.1:8080", "", "true", false},
		{"userinfo", "https://user:secret@proxy.example", "", "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(channeltype.JinaAllowLoopbackHTTPEnv, tc.optIn)
			transport := &jinaSecurityTransport{}
			previous := client.HTTPClient
			client.HTTPClient = &http.Client{Transport: transport}
			t.Cleanup(func() { client.HTTPClient = previous })
			c := requestContext(`{"model":"jina-embeddings-v3","input":"hello"}`)
			gmw.SetLogger(c, logger.Logger)
			m := &meta.Meta{Mode: relaymode.Embeddings, ChannelType: channeltype.Jina, BaseURL: tc.base, APIKey: "channel-secret", ActualModelName: "jina-embeddings-v3", Config: dbmodel.ChannelConfig{EndpointURLs: map[string]string{"embeddings": tc.override}}}
			resp, err := (&Adaptor{}).DoRequest(c, m, strings.NewReader(`{}`))
			if !tc.allowed {
				require.Error(t, err)
				require.Nil(t, resp)
				require.Empty(t, transport.requests)
				require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
				require.NotContains(t, err.Error(), "channel-secret")
				return
			}
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Len(t, transport.requests, 1)
			require.Equal(t, "Bearer channel-secret", transport.requests[0].Header.Get("Authorization"))
			if tc.override != "" {
				require.Equal(t, tc.override, transport.requests[0].URL.String())
			}
		})
	}
	t.Setenv(channeltype.JinaAllowLoopbackHTTPEnv, "false")
	c := requestContext(`{}`)
	c.Request.Header.Set("Authorization", "Bearer caller-secret")
	c.Request.Header.Set("X-Test", "caller-value")
	req, err := http.NewRequest(http.MethodPost, "http://proxy.example", nil)
	require.NoError(t, err)
	require.Error(t, (&Adaptor{}).SetupRequestHeader(c, req, &meta.Meta{APIKey: "channel-secret"}))
	require.Empty(t, req.Header, "validation must happen before copying any caller or provider headers")
}

// TestJinaRedirectsNeverReplayPaidRequests covers every redirect status Go can
// automatically follow. Jina's local client policy must not mutate the shared
// client's behavior for other adaptors, even for a same-host HTTPS redirect.
func TestJinaRedirectsNeverReplayPaidRequests(t *testing.T) {
	configureJinaSecurityTracing(t)
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, target := range []string{"https://api.jina.ai/redirected", "http://api.jina.ai/redirected", "https://other.example/redirected"} {
			transport := &jinaSecurityTransport{status: status, location: target}
			shared := &http.Client{Transport: transport}
			previous := client.HTTPClient
			client.HTTPClient = shared
			c := requestContext(`{}`)
			gmw.SetLogger(c, logger.Logger)
			m := &meta.Meta{Mode: relaymode.Embeddings, ChannelType: channeltype.Jina, APIKey: "channel-secret"}
			_, err := (&Adaptor{}).DoRequest(c, m, strings.NewReader(`{}`))
			client.HTTPClient = previous
			require.Error(t, err)
			require.Len(t, transport.requests, 1, "status=%d target=%s", status, target)
			require.Nil(t, shared.CheckRedirect, "provider policy must not mutate the shared client")
		}
	}
	transport := &jinaSecurityTransport{status: 307, location: "https://other.example/final"}
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{Transport: transport}
	defer func() { client.HTTPClient = previous }()
	c := requestContext(`{}`)
	gmw.SetLogger(c, logger.Logger)
	req, err := http.NewRequest(http.MethodPost, "https://other.example/start", strings.NewReader(`{}`))
	require.NoError(t, err)
	resp, err := adaptor.DoRequest(c, req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Len(t, transport.requests, 2, "non-Jina dispatch retains its original redirect policy")
}
