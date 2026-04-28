package openai

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	rmodel "github.com/Laisky/one-api/relay/model"
)

// TestMaybeParseRealtimeUsage verifies basic usage extraction from realtime events.
func TestMaybeParseRealtimeUsage(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"response.done","response":{"id":"resp_rt_1","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`)
	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
}

// TestMaybeParseRealtimeUsageDeduplicate verifies that repeated events for the
// same response ID are only counted once.
func TestMaybeParseRealtimeUsageDeduplicate(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"response.done","response":{"id":"resp_rt_dedup","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`)

	maybeParseRealtimeUsage(msg, usage, counted)
	maybeParseRealtimeUsage(msg, usage, counted) // duplicate

	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
	require.Len(t, counted, 1)
}

// TestMaybeParseRealtimeUsageMultipleResponses verifies usage from different
// response IDs are accumulated correctly.
func TestMaybeParseRealtimeUsageMultipleResponses(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg1 := []byte(`{"type":"response.done","response":{"id":"resp_rt_a","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`)
	msg2 := []byte(`{"type":"response.done","response":{"id":"resp_rt_b","usage":{"input_tokens":5,"output_tokens":15,"total_tokens":20}}}`)

	maybeParseRealtimeUsage(msg1, usage, counted)
	maybeParseRealtimeUsage(msg2, usage, counted)

	require.Equal(t, 15, usage.PromptTokens)
	require.Equal(t, 35, usage.CompletionTokens)
	require.Equal(t, 50, usage.TotalTokens)
	require.Len(t, counted, 2)
}

// TestMaybeParseRealtimeUsageNilSafety verifies nil inputs don't panic.
func TestMaybeParseRealtimeUsageNilSafety(t *testing.T) {
	t.Parallel()

	counted := map[string]struct{}{}
	// nil usage
	maybeParseRealtimeUsage([]byte(`{"response":{"id":"r","usage":{"input_tokens":1}}}`), nil, counted)

	// empty msg
	usage := &rmodel.Usage{}
	maybeParseRealtimeUsage(nil, usage, counted)
	maybeParseRealtimeUsage([]byte{}, usage, counted)

	// invalid json
	maybeParseRealtimeUsage([]byte(`{invalid`), usage, counted)

	require.Equal(t, 0, usage.PromptTokens)
}

// TestMaybeParseRealtimeUsageNoResponse verifies events without response object are ignored.
func TestMaybeParseRealtimeUsageNoResponse(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"session.created","session":{"id":"sess_1"}}`)
	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
}

// TestMaybeParseRealtimeUsageNoResponseID verifies events without response ID
// are still counted (backward compatibility) but won't be deduplicated.
func TestMaybeParseRealtimeUsageNoResponseID(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"response.done","response":{"usage":{"input_tokens":10,"output_tokens":20}}}`)
	maybeParseRealtimeUsage(msg, usage, counted)
	maybeParseRealtimeUsage(msg, usage, counted)

	// Without response ID, deduplication doesn't apply, so tokens are counted twice
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 40, usage.CompletionTokens)
}

// TestNegotiateRealtimeSubprotocols verifies subprotocol filtering.
func TestNegotiateRealtimeSubprotocols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		header   string
		expected []string
	}{
		{
			name:     "empty",
			header:   "",
			expected: nil,
		},
		{
			name:     "realtime only",
			header:   "realtime",
			expected: []string{"realtime"},
		},
		{
			name:     "full browser auth",
			header:   "realtime, openai-insecure-api-key.sk-test123, openai-beta.realtime-v1",
			expected: []string{"realtime", "openai-beta.realtime-v1"},
		},
		{
			name:     "only auth key",
			header:   "openai-insecure-api-key.sk-secret",
			expected: nil,
		},
		{
			name:     "multiple protocols without auth",
			header:   "realtime, openai-beta.realtime-v1",
			expected: []string{"realtime", "openai-beta.realtime-v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}}
			if tt.header != "" {
				r.Header.Set("Sec-WebSocket-Protocol", tt.header)
			}
			result := negotiateRealtimeSubprotocols(r)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestMaybeParseRealtimeUsageAudioDetails verifies that audio/text token
// details from input_token_details and output_token_details are parsed.
func TestMaybeParseRealtimeUsageAudioDetails(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{
		"type": "response.done",
		"response": {
			"id": "resp_audio_1",
			"usage": {
				"input_tokens": 500,
				"output_tokens": 300,
				"total_tokens": 800,
				"input_token_details": {
					"cached_tokens": 50,
					"text_tokens": 100,
					"audio_tokens": 400
				},
				"output_token_details": {
					"text_tokens": 50,
					"audio_tokens": 250
				}
			}
		}
	}`)

	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 500, usage.PromptTokens)
	require.Equal(t, 300, usage.CompletionTokens)
	require.Equal(t, 800, usage.TotalTokens)

	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 50, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 100, usage.PromptTokensDetails.TextTokens)
	require.Equal(t, 400, usage.PromptTokensDetails.AudioTokens)

	require.NotNil(t, usage.CompletionTokensDetails)
	require.Equal(t, 50, usage.CompletionTokensDetails.TextTokens)
	require.Equal(t, 250, usage.CompletionTokensDetails.AudioTokens)
}

// TestMaybeParseRealtimeUsageAudioAccumulation verifies that audio details
// from multiple response.done events are accumulated correctly.
func TestMaybeParseRealtimeUsageAudioAccumulation(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg1 := []byte(`{"type":"response.done","response":{"id":"r1","usage":{
		"input_tokens":100,"output_tokens":50,
		"input_token_details":{"audio_tokens":80,"text_tokens":20},
		"output_token_details":{"audio_tokens":40,"text_tokens":10}
	}}}`)
	msg2 := []byte(`{"type":"response.done","response":{"id":"r2","usage":{
		"input_tokens":200,"output_tokens":100,
		"input_token_details":{"audio_tokens":150,"text_tokens":50},
		"output_token_details":{"audio_tokens":70,"text_tokens":30}
	}}}`)

	maybeParseRealtimeUsage(msg1, usage, counted)
	maybeParseRealtimeUsage(msg2, usage, counted)

	require.Equal(t, 300, usage.PromptTokens)
	require.Equal(t, 150, usage.CompletionTokens)

	require.Equal(t, 230, usage.PromptTokensDetails.AudioTokens)     // 80+150
	require.Equal(t, 70, usage.PromptTokensDetails.TextTokens)       // 20+50
	require.Equal(t, 110, usage.CompletionTokensDetails.AudioTokens) // 40+70
	require.Equal(t, 40, usage.CompletionTokensDetails.TextTokens)   // 10+30
}

// TestMaybeParseRealtimeUsageNoDetails verifies that events without
// token details still parse totals correctly (backward compatibility).
func TestMaybeParseRealtimeUsageNoDetails(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"response.done","response":{"id":"r_nodetails","usage":{
		"input_tokens":100,"output_tokens":50,"total_tokens":150
	}}}`)

	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 50, usage.CompletionTokens)
	require.Nil(t, usage.PromptTokensDetails, "should be nil when no details provided")
	require.Nil(t, usage.CompletionTokensDetails, "should be nil when no details provided")
}

// TestCopyWSRealtimeUsageFullEvent verifies a realistic realtime response.done
// event with nested usage is parsed correctly.
func TestCopyWSRealtimeUsageFullEvent(t *testing.T) {
	t.Parallel()

	fullEvent := map[string]any{
		"type":     "response.done",
		"event_id": "event_abc",
		"response": map[string]any{
			"id":     "resp_full_1",
			"object": "realtime.response",
			"status": "completed",
			"usage": map[string]any{
				"input_tokens":  150,
				"output_tokens": 75,
				"total_tokens":  225,
			},
		},
	}
	msg, _ := json.Marshal(fullEvent)

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}
	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 150, usage.PromptTokens)
	require.Equal(t, 75, usage.CompletionTokens)
	require.Equal(t, 225, usage.TotalTokens)
}
