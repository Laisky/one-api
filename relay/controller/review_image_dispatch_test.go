package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestNanoInvalidImageNeverDispatches exercises the controller boundary before the native provider request.
func TestNanoInvalidImageNeverDispatches(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		valid     bool
	}{
		{"invalid", "data:invalid", false},
		{"valid", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 100000
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			var calls atomic.Int32
			var providerBody atomic.Value
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				data, readErr := io.ReadAll(r.Body)
				if readErr != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				providerBody.Store(string(data))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
			}))
			t.Cleanup(srv.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = srv.Client()
			t.Cleanup(func() { client.HTTPClient = oldClient })
			body := fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":%q}}]}],"max_tokens":1}`, tc.url)
			c, _, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", "/v1/chat/completions", body, srv.URL, balance, 1, false, &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			err := RelayTextHelper(c)
			drainCriticalTasks(t)
			if !tc.valid {
				require.Zero(t, calls.Load(), "invalid image must not reach the upstream provider")
				require.NotNil(t, err)
				require.Equal(t, http.StatusBadRequest, err.StatusCode)
				require.Zero(t, calls.Load())
				require.NotContains(t, err.Error.Message, tc.url)
				return
			}
			require.Nil(t, err)
			require.EqualValues(t, 1, calls.Load())
			require.NotContains(t, providerBody.Load().(string), "imageConfig")
		})
	}
}

// TestNanoMissing1KTariffRejectedBeforeDispatch prevents silently waiving the render fee.
func TestNanoMissing1KTariffRejectedBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prices map[string]float64
		valid  bool
	}{
		{"missing_1k", map[string]float64{"2048x2048": 1.5}, false},
		{"configured_1k", map[string]float64{"1024x1024": 1, "2048x2048": 1.5}, true},
		{"explicit_1k_override", map[string]float64{"2048x2048": 1.5}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 100000
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			var calls atomic.Int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
			}))
			t.Cleanup(srv.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = srv.Client()
			t.Cleanup(func() { client.HTTPClient = oldClient })
			body := `{"model":"alias","messages":[{"role":"user","content":"draw"}],"max_tokens":1}`
			cfg := &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: 0.0336, DefaultSize: "2048x2048", DefaultQuality: "standard", SizeMultipliers: tc.prices}}
			c, _, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", "/v1/chat/completions", body, srv.URL, balance, 1, false, cfg)
			if tc.name == "explicit_1k_override" {
				channel := c.MustGet(ctxkey.ChannelModel).(*dbmodel.Channel)
				require.NoError(t, channel.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{"gemini-nano-banana-2.1": *cfg, "$image-tier:gemini-nano-banana-2.1|size=1024x1024": {Ratio: 0.8}}))
			}
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			if !tc.valid {
				require.Zero(t, calls.Load())
				require.NotNil(t, apiErr)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

// TestNanoMissing1KTariffResponses checks the same pre-dispatch guard through the Responses fallback.
func TestNanoMissing1KTariffResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prices map[string]float64
		valid  bool
	}{
		{"missing_1k", map[string]float64{"2048x2048": 1.5}, false},
		{"configured_1k", map[string]float64{"1024x1024": 1, "2048x2048": 1.5}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 100000
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			var calls atomic.Int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
			}))
			t.Cleanup(srv.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = srv.Client()
			t.Cleanup(func() { client.HTTPClient = oldClient })
			body := `{"model":"alias","input":"draw","max_output_tokens":1}`
			cfg := &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: 0.0336, DefaultSize: "2048x2048", DefaultQuality: "standard", SizeMultipliers: tc.prices}}
			c, _, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", "/v1/responses", body, srv.URL, balance, 1, false, cfg)
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			if !tc.valid {
				require.Zero(t, calls.Load())
				require.NotNil(t, apiErr)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

// TestNanoMissing1KTariffClaudeMessages checks native tariff admission on the third public message format.
func TestNanoMissing1KTariffClaudeMessages(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prices map[string]float64
		valid  bool
	}{
		{"missing_1k", map[string]float64{"2048x2048": 1.5}, false},
		{"configured_1k", map[string]float64{"1024x1024": 1, "2048x2048": 1.5}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 100000
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			var calls atomic.Int32
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
			}))
			t.Cleanup(srv.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = srv.Client()
			t.Cleanup(func() { client.HTTPClient = oldClient })
			body := `{"model":"alias","max_tokens":1,"messages":[{"role":"user","content":"draw"}]}`
			cfg := &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: 0.0336, DefaultSize: "2048x2048", DefaultQuality: "standard", SizeMultipliers: tc.prices}}
			c, _, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", "/v1/messages", body, srv.URL, balance, 1, false, cfg)
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			if !tc.valid {
				require.Zero(t, calls.Load())
				require.NotNil(t, apiErr)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

// TestClaudeNanoImageOutputPreserved checks that image output is not silently dropped in Claude Messages conversion.
func TestClaudeNanoImageOutputPreserved(t *testing.T) {
	const balance int64 = 100000
	xaiVideoSetup(t, balance, false)
	oldPre := config.PreConsumedQuota
	config.PreConsumedQuota = 20
	t.Cleanup(func() { config.PreConsumedQuota = oldPre })
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hello"},{"inlineData":{"mimeType":"image/png","data":"`+png+`"}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1121,"totalTokenCount":1122,"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":1},{"modality":"IMAGE","tokenCount":1120}]}}`)
	}))
	t.Cleanup(srv.Close)
	oldClient := client.HTTPClient
	client.HTTPClient = srv.Client()
	t.Cleanup(func() { client.HTTPClient = oldClient })
	body := `{"model":"alias","max_tokens":1,"messages":[{"role":"user","content":"draw"}]}`
	c, rec, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", "/v1/messages", body, srv.URL, balance, 1, false, &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	require.Nil(t, apiErr)
	require.Contains(t, rec.Body.String(), "hello")
	require.True(t, strings.Contains(rec.Body.String(), png), "upstream image must not be silently dropped")
}
