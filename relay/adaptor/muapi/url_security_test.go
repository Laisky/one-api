package muapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMuAPIURLSegmentsCannotChangeRoute checks actual HTTP dispatch boundaries,
// including the single-dot segments that upstream proxies may canonicalize.
func TestMuAPIURLSegmentsCannotChangeRoute(t *testing.T) {
	for _, operation := range []string{"quote", "submit", "poll", "legacy_submit", "legacy_poll"} {
		for _, segment := range []string{".", "..", "../admin", "%2e%2e", "a/b", "a\\b", "x?target=evil", "x#fragment", "//evil.example", "a%2fb", "x\r\nHost: evil"} {
			t.Run(operation+"/"+segment, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"cost": 1, "currency": "USD", "request_id": segment, "status": "processing",
					}))
				}))
				defer server.Close()
				err := exerciseMuAPIURLOperation(t, operation, server.URL, segment)
				if operation == "legacy_poll" && strings.Contains(segment, "?") {
					// This API takes a request URI, not an opaque ID: a real
					// query is ignored by design and is not path injection.
					require.NoError(t, err)
					require.Zero(t, calls.Load())
					return
				}
				require.Error(t, err)
				require.Zero(t, calls.Load(), "invalid path segment must never be dispatched")
			})
		}
	}
}

// TestMuAPIBaseURLRejectsAmbiguousComponents proves that provider credentials
// never accompany query/fragment/userinfo-tainted or dot-segment base URLs.
func TestMuAPIBaseURLRejectsAmbiguousComponents(t *testing.T) {
	for _, operation := range []string{"quote", "submit", "poll", "legacy_submit", "legacy_poll"} {
		for _, component := range []string{"query", "empty_query", "fragment", "empty_fragment", "userinfo", "dot_path", "encoded_dot", "encoded_slash", "double_slash"} {
			t.Run(operation+"/"+component, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, err := io.WriteString(w, `{ "cost":1, "currency":"USD", "request_id":"safe-model", "status":"processing" }`)
					require.NoError(t, err)
				}))
				defer server.Close()
				base := server.URL
				switch component {
				case "query":
					base += "/v1?route=/admin"
				case "empty_query":
					base += "/v1?"
				case "fragment":
					base += "/v1#discard-api-path"
				case "empty_fragment":
					base += "/v1#"
				case "userinfo":
					base = strings.Replace(base, "://", "://user:password@", 1)
				case "dot_path":
					base += "/proxy/../admin"
				case "encoded_dot":
					base += "/proxy/%2e%2e/admin"
				case "encoded_slash":
					base += "/proxy%2fadmin"
				case "double_slash":
					base += "/proxy//admin"
				}
				require.Error(t, exerciseMuAPIURLOperation(t, operation, base, "safe-model"))
				require.Zero(t, calls.Load(), "invalid provider origin/path must fail before network I/O")
			})
		}
	}
}

// TestMuAPIURLRetainsConfiguredProxyPrefix keeps explicit administrator routing
// compatible while untrusted model and task identifiers remain one segment.
func TestMuAPIURLRetainsConfiguredProxyPrefix(t *testing.T) {
	for _, base := range []string{"https://api.muapi.ai", "https://api.muapi.ai/v1", "https://api.muapi.ai/api/v1", "https://gateway.example/proxy/muapi/api/v1/"} {
		root := "https://api.muapi.ai"
		if strings.Contains(base, "gateway.example") {
			root = "https://gateway.example/proxy/muapi"
		}
		got, err := (&Adaptor{}).GetRequestURL(&meta.Meta{Mode: relaymode.MuAsyncVideos, BaseURL: base,
			ActualModelName: "new-model.v3_1", RequestURLPath: "/v1/async/videos"})
		require.NoError(t, err)
		require.Equal(t, root+"/api/v1/new-model.v3_1", got)
	}
}

// exerciseMuAPIURLOperation invokes each shipped provider entry point and
// returns its validation error. All HTTP traffic targets local test fixtures.
func exerciseMuAPIURLOperation(t *testing.T, operation, base, segment string) error {
	t.Helper()
	info := &meta.Meta{Mode: relaymode.MuAsyncVideos, BaseURL: base, APIKey: "test-only-key",
		ActualModelName: segment, RequestURLPath: "/v1/async/videos"}
	a := &Adaptor{}
	switch operation {
	case "quote":
		c := newMuAPITestContext(http.MethodPost, "/v1/async/videos", `{"duration":5}`)
		bindMuAPIQuoteTestChannel(c, info)
		_, err := a.EstimateVideoCostUSD(c, info, &model.VideoRequest{Duration: float64Ptr(5)})
		return err
	case "submit":
		_, err := a.SubmitVideo(context.Background(), info, []byte(`{"duration":5}`))
		return err
	case "poll":
		_, err := a.PollVideo(context.Background(), info, segment)
		return err
	case "legacy_poll":
		info.RequestURLPath += "/" + segment
		_, err := a.GetRequestURL(info)
		return err
	default:
		_, err := a.GetRequestURL(info)
		return err
	}
}

// TestMuAPIURLHTTPContract checks the actual URL and credential boundary for
// each operation and supported base form, including a configured proxy prefix.
func TestMuAPIURLHTTPContract(t *testing.T) {
	for _, operation := range []string{"quote", "submit", "poll"} {
		for _, suffix := range []string{"", "/v1", "/api/v1", "/proxy/muapi/api/v1/"} {
			t.Run(operation+suffix, func(t *testing.T) {
				var path, key, authorization, query string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path, key, authorization, query = r.URL.Path, r.Header.Get("x-api-key"), r.Header.Get("Authorization"), r.URL.RawQuery
					w.Header().Set("Content-Type", "application/json")
					body := `{"request_id":"new-model.v3_1","status":"processing","cost":{"amount_usd":0.42}}`
					if operation == "quote" {
						body = `{"cost":0.42,"currency":"USD"}`
					}
					_, err := io.WriteString(w, body)
					require.NoError(t, err)
				}))
				defer server.Close()
				require.NoError(t, exerciseMuAPIURLOperation(t, operation, server.URL+suffix, "new-model.v3_1"))
				prefix := "/api/v1"
				if strings.Contains(suffix, "proxy") {
					prefix = "/proxy/muapi" + prefix
				}
				want := prefix + "/new-model.v3_1"
				if operation == "quote" {
					want = prefix + "/models/new-model.v3_1/estimate-cost"
				} else if operation == "poll" {
					want = prefix + "/predictions/new-model.v3_1/result"
				}
				require.Equal(t, want, path)
				require.Equal(t, "test-only-key", key)
				require.Empty(t, authorization)
				require.Empty(t, query)
			})
		}
	}
}
