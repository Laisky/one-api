package openai

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
)

// responseWSUpstreamURLForQuery resolves the Responses WebSocket upstream URL
// for a caller query. Parameters: t is the test handle, baseURL and
// actualModel describe the channel binding, and rawQuery is the caller's raw
// query string. Returns: the parsed upstream URL.
func responseWSUpstreamURLForQuery(t *testing.T, baseURL, actualModel, rawQuery string) *url.URL {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	ctx.Request.URL.RawQuery = rawQuery
	raw, _, err := resolveResponseAPIWebSocketUpstreamURL(ctx, &rmeta.Meta{BaseURL: baseURL, ActualModelName: actualModel})
	require.NoError(t, err)
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

// TestResponseAPIWebSocketUpstreamQueryPolicy verifies the Responses
// WebSocket handshake no longer relays the caller's raw query. OpenAI
// documents no query parameters for wss://.../v1/responses (the model travels
// in each response.create frame), so only the gateway's own `model` routing
// hint survives, rewritten to the mapped model it was bound to.
// Parameters: t is the test handle. Returns: none.
func TestResponseAPIWebSocketUpstreamQueryPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		rawQuery    string
		actualModel string
		wantQuery   url.Values
	}{
		{
			name:      "no_query_stays_empty",
			wantQuery: url.Values{},
		},
		{
			name:        "routing_hint_is_rewritten_to_the_mapped_model",
			rawQuery:    "model=my-alias",
			actualModel: "gpt-5-mini",
			wantQuery:   url.Values{"model": {"gpt-5-mini"}},
		},
		{
			name:        "caller_parameters_are_dropped",
			rawQuery:    "model=my-alias&Model=gpt-5-pro&api-version=2025-04-01-preview&deployment=gpt-5-pro&service_tier=priority&foo=bar",
			actualModel: "gpt-5-mini",
			wantQuery:   url.Values{"model": {"gpt-5-mini"}},
		},
		{
			name:      "unbound_handshake_forwards_nothing",
			rawQuery:  "foo=bar&model=",
			wantQuery: url.Values{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed := responseWSUpstreamURLForQuery(t, "https://api.openai.com", tc.actualModel, tc.rawQuery)
			require.Equal(t, "wss", parsed.Scheme)
			require.Equal(t, "api.openai.com", parsed.Host)
			require.Equal(t, "/v1/responses", parsed.Path)
			require.Equal(t, tc.wantQuery, parsed.Query())
		})
	}
}

// TestResponseAPIWebSocketUpstreamQueryCannotRedirectHandshake is the control
// for the go/request-forgery review: the caller's query never changes the
// upstream scheme, host, credentials, path or fragment. Parameters: t is the
// test handle. Returns: none.
func TestResponseAPIWebSocketUpstreamQueryCannotRedirectHandshake(t *testing.T) {
	t.Parallel()
	for _, rawQuery := range []string{
		"model=x&@evil.example/",
		"model=x#@evil.example/path",
		"model=x&%2F..%2F..%2Fadmin=1",
		"model=x&next=//evil.example/v1/responses",
		"model=x&user:pass@evil.example=1",
		"%zz=1&model=x",
	} {
		parsed := responseWSUpstreamURLForQuery(t, "https://channel.example", "gpt-5-mini", rawQuery)
		require.Equal(t, "wss", parsed.Scheme, "query=%q", rawQuery)
		require.Equal(t, "channel.example", parsed.Host, "query=%q", rawQuery)
		require.Nil(t, parsed.User, "query=%q", rawQuery)
		require.Equal(t, "/v1/responses", parsed.Path, "query=%q", rawQuery)
		require.Empty(t, parsed.Fragment, "query=%q", rawQuery)
	}
}
