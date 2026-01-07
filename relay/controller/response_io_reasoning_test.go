package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/channeltype"
)

func TestNormalizeResponseAPIRawBody_NormalizesReasoningSummaryForOpenAI(t *testing.T) {
	raw := []byte(`{
		"model":"o4-mini",
		"input":"hi",
		"reasoning":{
			"summary":"concise",
			"generate_summary":"auto"
		}
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	patched, stats, changed, err := normalizeResponseAPIRawBody(raw, &req, channeltype.OpenAI)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, stats.ReasoningSummaryFixed)

	var root map[string]any
	require.NoError(t, json.Unmarshal(patched, &root))
	reasoning, ok := root["reasoning"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "detailed", reasoning["summary"])
	// Ensure we did not clobber other fields inside reasoning.
	require.Equal(t, "auto", reasoning["generate_summary"])
}

func TestNormalizeResponseAPIRawBody_DoesNotNormalizeReasoningSummaryForNonOpenAI(t *testing.T) {
	raw := []byte(`{
		"model":"o4-mini",
		"input":"hi",
		"reasoning":{
			"summary":"concise"
		}
	}`)

	var req openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &req))

	patched, stats, changed, err := normalizeResponseAPIRawBody(raw, &req, channeltype.XAI)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, 0, stats.ReasoningSummaryFixed)
	require.JSONEq(t, string(raw), string(patched))
}
