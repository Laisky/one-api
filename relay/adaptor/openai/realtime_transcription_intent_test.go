package openai

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
)

// TestRealtimeTranscriptionIntentDropsModelParameter pins the upstream URL for
// both realtime surfaces. Parameters: t is the test handle. Returns: none.
//
// Verified against the live API on 2026-09-18: `?intent=transcription` alone
// opens a transcription session, while adding any model — the routing model, a
// transcription model, or a realtime model — is rejected at the handshake with
// invalid_model ("You must not provide a model parameter for transcription
// sessions" / "not supported in transcription mode"). Callers must still name a
// model in their own query for routing and billing, so the proxy has to drop it
// rather than forward it; otherwise no transcription session can be opened.
func TestRealtimeTranscriptionIntentDropsModelParameter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		clientQuery string
		model       string
		wantQuery   url.Values
	}{
		{
			name:        "transcription_intent_drops_routing_model",
			clientQuery: "model=gpt-4o-transcribe&intent=transcription",
			model:       "gpt-4o-transcribe",
			wantQuery:   url.Values{"intent": {"transcription"}},
		},
		{
			name:        "transcription_intent_is_case_insensitive",
			clientQuery: "model=gpt-4o-transcribe&intent=Transcription",
			model:       "gpt-4o-transcribe",
			wantQuery:   url.Values{"intent": {"Transcription"}},
		},
		{
			name:        "conversation_keeps_mapped_model",
			clientQuery: "model=my-voice",
			model:       "gpt-realtime",
			wantQuery:   url.Values{"model": {"gpt-realtime"}},
		},
		{
			name:        "conversation_intent_keeps_mapped_model",
			clientQuery: "model=my-voice&intent=conversation",
			model:       "gpt-realtime",
			wantQuery:   url.Values{"model": {"gpt-realtime"}, "intent": {"conversation"}},
		},
		{
			name:        "unrelated_query_preserved",
			clientQuery: "model=my-voice&trace=abc",
			model:       "gpt-realtime",
			wantQuery:   url.Values{"model": {"gpt-realtime"}, "trace": {"abc"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := realtimeWebSocketUpstreamURL(&rmeta.Meta{ActualModelName: tc.model}, tc.clientQuery)
			parsed, err := url.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, "wss", parsed.Scheme)
			require.Equal(t, "api.openai.com", parsed.Host)
			require.Equal(t, "/v1/realtime", parsed.Path)
			require.Equal(t, tc.wantQuery, parsed.Query())
		})
	}
}
