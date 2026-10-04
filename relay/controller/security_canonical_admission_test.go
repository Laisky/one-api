package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// canonicalAdmissionWire is the exact mock-provider request, not an estimator-only model.
type canonicalAdmissionWire struct {
	Body relaymodel.GeneralOpenAIRequest
	Path string
	Err  error
}

// canonicalAdmissionServer records real outgoing requests and returns a small authoritative receipt.
func canonicalAdmissionServer(t *testing.T, missingPrompt bool) (*httptest.Server, *atomic.Int32, chan canonicalAdmissionWire) {
	t.Helper()
	calls := new(atomic.Int32)
	seen := make(chan canonicalAdmissionWire, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body relaymodel.GeneralOpenAIRequest
		err := json.NewDecoder(r.Body).Decode(&body)
		seen <- canonicalAdmissionWire{Body: body, Path: r.URL.Path, Err: err}
		usage := map[string]any{"completion_tokens": 1}
		if !missingPrompt {
			usage["prompt_tokens"] = 1
			usage["total_tokens"] = 2
		}
		w.Header().Set("Content-Type", "application/json")
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			event, _ := json.Marshal(map[string]any{"id": "synthetic-canonical", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "ok"}}}})
			fmt.Fprintf(w, "data: %s\n\n", event)
			event, _ = json.Marshal(map[string]any{"choices": []any{}, "usage": usage})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", event)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "synthetic-canonical", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}}, "usage": usage})
	}))
	t.Cleanup(server.Close)
	old := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = old })
	return server, calls, seen
}

// canonicalAdmissionConfiguration keeps test quotes deterministic and restores all globals.
func canonicalAdmissionConfiguration(t *testing.T) {
	t.Helper()
	oldPre, oldDefault, oldUsage := config.PreConsumedQuota, config.DefaultMaxToken, config.EnforceIncludeUsage
	config.PreConsumedQuota = 0
	config.DefaultMaxToken = 128
	config.EnforceIncludeUsage = false
	t.Cleanup(func() {
		config.PreConsumedQuota = oldPre
		config.DefaultMaxToken = oldDefault
		config.EnforceIncludeUsage = oldUsage
	})
}

// TestSecurityCanonicalLimitsHTTP proves conflicting and converted-default limits cannot buy underfunded work.
func TestSecurityCanonicalLimitsHTTP(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		channel               int
		raw, stream, defaults bool
	}{
		{name: "converted", channel: channeltype.OpenAICompatible},
		{name: "passthrough", channel: channeltype.OpenAICompatible, raw: true},
		{name: "native_openai", channel: channeltype.OpenAI},
		{name: "converted_stream", channel: channeltype.OpenAICompatible, stream: true},
		{name: "converted_default", channel: channeltype.OpenAICompatible, defaults: true},
		{name: "converted_zero_default", channel: channeltype.OpenAICompatible, defaults: true, stream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100)
			xaiVideoSetup(t, balance, false)
			canonicalAdmissionConfiguration(t)
			server, calls, seen := canonicalAdmissionServer(t, false)
			payload := map[string]any{"model": "alias", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "max_tokens": 500, "max_completion_tokens": 1, "stream": tc.stream}
			if tc.defaults {
				delete(payload, "max_tokens")
				delete(payload, "max_completion_tokens")
				if tc.stream {
					payload["max_completion_tokens"] = 0
				}
			}
			if tc.raw {
				payload["model"] = "gpt-4"
			}
			wire, err := json.Marshal(payload)
			require.NoError(t, err)
			c, _, id := protocolContext(t, tc.channel, "gpt-4", "/v1/chat/completions", string(wire), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			if tc.raw {
				c.Set(ctxkey.ModelMapping, map[string]string{})
				c.Set(ctxkey.RequestModel, "gpt-4")
			}
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			if calls.Load() > 0 {
				observed := <-seen
				require.NoError(t, observed.Err)
				t.Logf("REPRODUCED_493_LIMIT_DISPATCH path=%s max_tokens=%d max_completion_tokens=%v", observed.Path, observed.Body.MaxTokens, observed.Body.MaxCompletionTokens)
			}
			require.Zero(t, calls.Load(), "underfunded or ambiguous output must fail before provider work")
			require.NotNil(t, apiErr)
			if tc.defaults {
				require.Equal(t, 403, apiErr.StatusCode)
			} else {
				require.Equal(t, 400, apiErr.StatusCode)
			}
			require.Equal(t, balance, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance, token.RemainQuota)
			var rows int64
			require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&rows).Error)
			require.Zero(t, rows)
		})
	}
}

// TestSecurityCanonicalSchemaHTTP captures the injected schema instructions across a real mapped DeepSeek route.
func TestSecurityCanonicalSchemaHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			const balance = int64(10_000)
			xaiVideoSetup(t, balance, false)
			canonicalAdmissionConfiguration(t)
			server, calls, seen := canonicalAdmissionServer(t, false)
			description := strings.Repeat("detailed property guidance ", 20_000)
			payload := map[string]any{"model": "alias", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "max_tokens": 1, "stream": stream,
				"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string", "description": description}}}}}}
			wire, err := json.Marshal(payload)
			require.NoError(t, err)
			c, _, id := protocolContext(t, channeltype.DeepSeek, "deepseek-chat", "/v1/chat/completions", string(wire), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			if calls.Load() > 0 {
				observed := <-seen
				require.NoError(t, observed.Err)
				require.Nil(t, observed.Body.ResponseFormat)
				require.Contains(t, observed.Body.Messages[0].StringContent(), description)
				t.Log("REPRODUCED_493_SCHEMA_DISPATCH")
			}
			require.Zero(t, calls.Load(), "the schema actually sent upstream must be quoted before admission")
			require.NotNil(t, apiErr)
			require.Equal(t, 403, apiErr.StatusCode)
			require.Equal(t, balance, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance, token.RemainQuota)
			var rows int64
			require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&rows).Error)
			require.Zero(t, rows)
		})
	}
}

// TestSecurityCanonicalSchemaMissingReceipt bills the final injected prompt exactly once when input usage is absent.
func TestSecurityCanonicalSchemaMissingReceipt(t *testing.T) {
	const balance = int64(10_000_000)
	xaiVideoSetup(t, balance, false)
	canonicalAdmissionConfiguration(t)
	server, calls, seen := canonicalAdmissionServer(t, true)
	description := strings.Repeat("field guidance ", 300)
	payload := map[string]any{"model": "alias", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "max_tokens": 1,
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "description": description, "schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}}}}}}
	wire, err := json.Marshal(payload)
	require.NoError(t, err)
	c, _, id := protocolContext(t, channeltype.DeepSeek, "deepseek-chat", "/v1/chat/completions", string(wire), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	require.Nil(t, RelayTextHelper(c))
	drainCriticalTasks(t)
	require.EqualValues(t, 1, calls.Load())
	observed := <-seen
	require.NoError(t, observed.Err)
	require.Nil(t, observed.Body.ResponseFormat)
	require.Equal(t, 1, strings.Count(observed.Body.Messages[0].StringContent(), description), "schema must not be injected twice")
	prompt := getPromptTokens(context.Background(), &observed.Body, relaymode.ChatCompletions)
	expected := int64(prompt + 1)
	actual := requestCostQuota(t, id)
	if actual < expected {
		t.Log("REPRODUCED_493_SCHEMA_SETTLEMENT")
	}
	require.Equal(t, expected, actual)
	require.Equal(t, balance-expected, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance-expected, token.RemainQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.EqualValues(t, expected, logs[0].Quota)
}

// TestSecurityCanonicalCompatibleLimits preserves valid absent, zero, equal and single output limits.
func TestSecurityCanonicalCompatibleLimits(t *testing.T) {
	for _, limits := range []map[string]any{{}, {"max_tokens": 0, "max_completion_tokens": 0}, {"max_tokens": 3}, {"max_completion_tokens": 3}, {"max_tokens": 3, "max_completion_tokens": 3}, {"max_tokens": 0, "max_completion_tokens": 3}} {
		t.Run(fmt.Sprint(limits), func(t *testing.T) {
			const balance = int64(10_000)
			xaiVideoSetup(t, balance, false)
			canonicalAdmissionConfiguration(t)
			server, calls, seen := canonicalAdmissionServer(t, false)
			payload := map[string]any{"model": "alias", "messages": []any{map[string]any{"role": "user", "content": "hello"}}}
			for key, value := range limits {
				payload[key] = value
			}
			wire, err := json.Marshal(payload)
			require.NoError(t, err)
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4", "/v1/chat/completions", string(wire), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			require.Nil(t, RelayTextHelper(c))
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load())
			observed := <-seen
			require.NoError(t, observed.Err)
			require.Equal(t, "gpt-4", observed.Body.Model)
			require.NotNil(t, observed.Body.MaxCompletionTokens)
			require.EqualValues(t, 2, requestCostQuota(t, id))
			require.Equal(t, balance-2, reloadUserQuota(t))
		})
	}
}
