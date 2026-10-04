package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// providerCopyTransport confines all provider and polling requests to the local fixture.
type providerCopyTransport struct {
	target *url.URL
	next   http.RoundTripper
}

// RoundTrip rewrites only the expected synthetic upstream hosts and never dials a provider.
func (tr providerCopyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != tr.target.Host && r.URL.Host != "api.replicate.com" {
		return nil, errors.New("unexpected upstream host in provider-copy fixture")
	}
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Scheme, u.Host = tr.target.Scheme, tr.target.Host
	copy.URL, copy.Host = &u, tr.target.Host
	return tr.next.RoundTrip(copy)
}

// providerCopyObservation captures the actual HTTP limit and path rather than the quoting DTO.
type providerCopyObservation struct {
	limit int
	path  string
	err   error
}

// providerCopyServer returns a bounded, local-only provider fixture and observed POST requests.
func providerCopyServer(t *testing.T, replicate bool) (*httptest.Server, *atomic.Int32, chan providerCopyObservation) {
	t.Helper()
	calls := new(atomic.Int32)
	seen := make(chan providerCopyObservation, 1)
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/synthetic-task" {
			_, err := fmt.Fprint(w, `{"id":"synthetic-copy","status":"succeeded","output":"ok"}`)
			if err != nil {
				return
			}
			return
		}
		calls.Add(1)
		var wire struct {
			MaxTokens int `json:"max_tokens"`
			Input     struct {
				MaxTokens int `json:"max_tokens"`
			} `json:"input"`
		}
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&wire)
		limit := wire.MaxTokens
		if replicate {
			limit = wire.Input.MaxTokens
		}
		select {
		case seen <- providerCopyObservation{limit: limit, path: r.URL.Path, err: err}:
		default:
		}
		if replicate {
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(map[string]any{"id": "synthetic-copy", "urls": map[string]any{"get": server.URL + "/synthetic-task"}}); err != nil {
				return
			}
			return
		}
		if _, err := fmt.Fprint(w, `{"id":"synthetic-copy","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`); err != nil {
			return
		}
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	local := server.Client()
	local.Transport = providerCopyTransport{target: target, next: local.Transport}
	local.Timeout = 2 * time.Second
	oldRelay, oldDefault := client.HTTPClient, http.DefaultClient
	client.HTTPClient, http.DefaultClient = local, local
	t.Cleanup(func() { client.HTTPClient, http.DefaultClient = oldRelay, oldDefault })
	return server, calls, seen
}

// TestSecurityProviderCopyLimitsHTTP checks provider defaults and explicit limits through real admission and HTTP.
func TestSecurityProviderCopyLimitsHTTP(t *testing.T) {
	for _, provider := range []struct {
		name, actual string
		channel      int
		replicate    bool
	}{
		{"replicate", "meta/meta-llama-3.1-405b-instruct", channeltype.Replicate, true},
		{"anthropic", "claude-sonnet-4", channeltype.Anthropic, false},
	} {
		for _, tc := range []struct {
			name, limits string
			limit        int
			low, stream  bool
		}{
			{"absent_underfunded", "", 128, true, false},
			{"zero_underfunded", `,"max_tokens":0,"max_completion_tokens":0`, 128, true, false},
			{"stream_default_underfunded", "", 128, true, true},
			{"completion_only", `,"max_completion_tokens":1`, 1, false, false},
			{"legacy_only", `,"max_tokens":3`, 3, false, false},
			{"equal", `,"max_tokens":3,"max_completion_tokens":3`, 3, false, false},
			{"zero_legacy_completion", `,"max_tokens":0,"max_completion_tokens":3`, 3, false, false},
			{"funded_default", "", 128, false, false},
		} {
			t.Run(provider.name+"/"+tc.name, func(t *testing.T) {
				balance := int64(10_000)
				if tc.low || tc.limit == 1 {
					balance = 100
				}
				xaiVideoSetup(t, balance, false)
				canonicalAdmissionConfiguration(t)
				server, calls, seen := providerCopyServer(t, provider.replicate)
				body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"stream":%t%s}`, provider.actual, tc.stream, tc.limits)
				c, _, id := protocolContext(t, provider.channel, provider.actual, "/v1/chat/completions", body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				c.Set(ctxkey.ModelMapping, map[string]string{})
				c.Set(ctxkey.RequestModel, provider.actual)
				bounded, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
				defer cancel()
				c.Request = c.Request.WithContext(bounded)
				apiErr := RelayTextHelper(c)
				drainCriticalTasks(t)
				if calls.Load() > 0 {
					observed := <-seen
					require.NoError(t, observed.err)
					expectedPath := "/v1/messages"
					if provider.replicate {
						expectedPath = "/v1/models/" + provider.actual + "/predictions"
					}
					require.Equal(t, expectedPath, observed.path)
					t.Logf("PROVIDER_COPY_WIRE provider=%s requested=%s observed_limit=%d balance=%d", provider.name, tc.limits, observed.limit, balance)
					require.Equal(t, tc.limit, observed.limit, "provider must honor the explicit completion limit")
				}
				if tc.low {
					require.Zero(t, calls.Load(), "underfunded provider default must fail before dispatch")
					require.NotNil(t, apiErr)
					require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
					require.Equal(t, balance, reloadUserQuota(t))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, balance, token.RemainQuota)
					var costs int64
					require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costs).Error)
					require.Zero(t, costs)
					return
				}
				require.Nil(t, apiErr)
				require.EqualValues(t, 1, calls.Load())
			})
		}
	}
}
