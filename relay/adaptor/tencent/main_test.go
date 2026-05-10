package tencent

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestResponseTencent2OpenAI_EmptyChoicesEmitsArray verifies that an upstream
// response with no Choices still serializes "choices":[] (never null).
func TestResponseTencent2OpenAI_EmptyChoicesEmitsArray(t *testing.T) {
	t.Parallel()

	resp := &ChatResponse{
		ReqID: "req-empty",
	}

	out := responseTencent2OpenAI(resp)
	require.NotNil(t, out.Choices, "Choices must be non-nil")
	require.Len(t, out.Choices, 0)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"choices":[]`)
	require.NotContains(t, string(raw), `"choices":null`)
}

// TestResponseTencent2OpenAI_PopulatedRoundTrips sanity-checks that a normal
// upstream response continues to be converted correctly.
func TestResponseTencent2OpenAI_PopulatedRoundTrips(t *testing.T) {
	t.Parallel()

	resp := &ChatResponse{
		ReqID: "req-ok",
		Choices: []ResponseChoices{
			{
				FinishReason: "stop",
				Messages:     Message{Role: "assistant", Content: "hello"},
			},
		},
		Usage: Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
	}

	out := responseTencent2OpenAI(resp)
	require.Len(t, out.Choices, 1)
	require.Equal(t, "hello", out.Choices[0].Message.StringContent())
	require.Equal(t, "stop", out.Choices[0].FinishReason)
	require.Equal(t, 3, out.Usage.TotalTokens)
}

// TestStreamResponseTencent2OpenAI_EmptyDeltaEmitsArray verifies that an empty
// upstream stream chunk is serialized with "choices":[] and not null.
func TestStreamResponseTencent2OpenAI_EmptyDeltaEmitsArray(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	out := streamResponseTencent2OpenAI(c, &ChatResponse{})
	require.NotNil(t, out.Choices, "stream Choices must be non-nil")
	require.Len(t, out.Choices, 0)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.True(t, bytes.Contains(raw, []byte(`"choices":[]`)))
	require.False(t, bytes.Contains(raw, []byte(`"choices":null`)))
}

// TestStreamResponseTencent2OpenAI_PopulatedDeltaSerializes is a sanity check for
// a normal stream delta carrying a single choice.
func TestStreamResponseTencent2OpenAI_PopulatedDeltaSerializes(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	in := &ChatResponse{
		Choices: []ResponseChoices{
			{
				FinishReason: "stop",
				Delta:        Message{Role: "assistant", Content: "hi"},
			},
		},
	}

	out := streamResponseTencent2OpenAI(c, in)
	require.Len(t, out.Choices, 1)
	require.Equal(t, "hi", out.Choices[0].Delta.StringContent())
	require.NotNil(t, out.Choices[0].FinishReason)
	require.Equal(t, "stop", *out.Choices[0].FinishReason)
}
