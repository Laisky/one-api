package controller

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityLyriaRejectsUnboundedRequests verifies unsupported multiplicity and
// tool loops never reach a paid provider. Parameters: t owns the fixture. Returns:
// none; ordinary one-generation requests remain covered by the positive suite.
func TestSecurityLyriaRejectsUnboundedRequests(t *testing.T) {
	for _, extra := range []string{`"n":2`, `"n":null`, `"extra_body":{"n":2}`, `"candidateCount":2`, `"tools":[{"type":"function","function":{"name":"lookup"}}]`} {
		t.Run(extra, func(t *testing.T) {
			xaiVideoSetup(t, 100000, false)
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(upstream.Close)
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			body := `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128,` + extra + `}`
			c, _, _ := protocolContext(t, channeltype.OpenRouter, "google/lyria-3-clip-preview", "/v1/chat/completions", body, upstream.URL+"/v1", 100000, 1, false, nil)
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Zero(t, calls.Load())
			require.EqualValues(t, 100000, reloadUserQuota(t))
		})
	}
}

// TestSecurityMediaTariffUnknownAndExpiredNeverDispatch exercises the real
// admission path when a catalog snapshot cannot authorize free work. Parameters:
// t owns the fixture. Returns: none; static catalog changes are isolated per case.
func TestSecurityMediaTariffUnknownAndExpiredNeverDispatch(t *testing.T) {
	const name = "google/lyria-3-clip-preview"
	for _, state := range []string{"unknown", "contract", "expired", "missing_generation_price"} {
		t.Run(state, func(t *testing.T) {
			xaiVideoSetup(t, 100000, false)
			original := openrouter.ModelRatios[name]
			cfg := original.Clone()
			switch state {
			case "expired":
				cfg.PricingProvenance.ValidUntil = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			case "missing_generation_price":
				cfg.PerCall = nil
			default:
				cfg.PricingProvenance.State = state
			}
			openrouter.ModelRatios[name] = cfg
			t.Cleanup(func() { openrouter.ModelRatios[name] = original })
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(upstream.Close)
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c, _, _ := protocolContext(t, channeltype.OpenRouter, name, "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128}`, upstream.URL+"/v1", 100000, 1, false, nil)
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Zero(t, calls.Load())
			require.EqualValues(t, 100000, reloadUserQuota(t))
		})
	}
}
