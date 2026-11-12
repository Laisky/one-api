package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestNativePartTextPresenceWire distinguishes explicit empty text, metadata-only parts and function-call parts.
func TestNativePartTextPresenceWire(t *testing.T) {
	var part Part
	for _, raw := range []string{`{"text":""}`, `{"thoughtSignature":"c2ln"}`, `{"text":"","thoughtSignature":"c2ln"}`, `{"functionCall":{"name":"lookup","args":{}}}`} {
		require.NoError(t, json.Unmarshal([]byte(raw), &part))
		encoded, err := json.Marshal(part)
		require.NoError(t, err)
		require.JSONEq(t, raw, string(encoded))
	}
	generated, err := json.Marshal(Part{TextPresent: true})
	require.NoError(t, err)
	require.JSONEq(t, `{"text":""}`, string(generated))
	call, err := json.Marshal(Part{FunctionCall: &FunctionCall{FunctionName: "lookup", Arguments: map[string]any{}}})
	require.NoError(t, err)
	require.NotContains(t, string(call), `"text"`)
}

// TestGeminiMetadataOnlyPartRoundTrip keeps signature-only native parts distinct from explicit signed empty text.
func TestGeminiMetadataOnlyPartRoundTrip(t *testing.T) {
	for _, rawPart := range []string{`{"thoughtSignature":"c2ln"}`, `{"text":"","thoughtSignature":"c2ln"}`} {
		c, recorder := signatureTestContext()
		payload := `{"candidates":[{"content":{"parts":[` + rawPart + `]},"finishReason":"STOP"}]}`
		failure, _ := Handler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}, 0, "gemini-3.8-flash")
		require.Nil(t, failure)
		var response struct {
			Choices []struct {
				Message model.Message `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		converted, err := ConvertNativeRequestChecked(c, model.GeneralOpenAIRequest{Model: "gemini-3.8-flash", Messages: []model.Message{response.Choices[0].Message}})
		require.NoError(t, err)
		encoded, err := json.Marshal(converted.Contents[0].Parts[0])
		require.NoError(t, err)
		require.JSONEq(t, rawPart, string(encoded))
	}
}
