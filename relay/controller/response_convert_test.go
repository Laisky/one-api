package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

func TestRenderChatResponseAsResponseAPIUsesOriginModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	request := &openai.ResponseAPIRequest{Model: "public-alias"}
	meta := &metalib.Meta{OriginModelName: "public-alias", ActualModelName: "hidden-target"}
	textResp := &openai_compatible.SlimTextResponse{}

	require.NoError(t, renderChatResponseAsResponseAPI(c, http.StatusOK, textResp, request, meta))
	require.Equal(t, http.StatusOK, w.Code)

	var response openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, "public-alias", response.Model)
}

// TestBuildResponseOutput_EmptyChoicesReturnsEmptySlice verifies that buildResponseOutput
// returns a non-nil zero-length slice when no choices yield content. The Responses API
// schema marks "output" as a required array; emitting null breaks strict SDK clients.
func TestBuildResponseOutput_EmptyChoicesReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		choices []openai_compatible.TextResponseChoice
	}{
		{name: "nil choices", choices: nil},
		{name: "empty choices slice", choices: []openai_compatible.TextResponseChoice{}},
		{
			name: "single choice with empty content and no tool calls",
			choices: []openai_compatible.TextResponseChoice{
				{Message: relaymodel.Message{Role: "assistant", Content: ""}, FinishReason: "content_filter"},
			},
		},
		{
			name: "single choice with whitespace-only content",
			choices: []openai_compatible.TextResponseChoice{
				{Message: relaymodel.Message{Role: "assistant", Content: "   \n\t  "}},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := buildResponseOutput(tc.choices)
			require.NotNil(t, out, "buildResponseOutput must never return nil")
			require.Equal(t, 0, len(out), "expected empty slice for empty/contentless input")
		})
	}
}

// TestBuildResponseOutput_PopulatedReturnsItems verifies a normal completion produces
// the expected output items (sanity check that the empty-slice fix didn't regress the
// happy path).
func TestBuildResponseOutput_PopulatedReturnsItems(t *testing.T) {
	t.Parallel()

	choices := []openai_compatible.TextResponseChoice{
		{
			Message:      relaymodel.Message{Role: "assistant", Content: "hello world"},
			FinishReason: "stop",
		},
	}
	out := buildResponseOutput(choices)
	require.NotNil(t, out)
	require.Len(t, out, 1)
	assert.Equal(t, "message", out[0].Type)
	assert.Equal(t, "assistant", out[0].Role)
	require.Len(t, out[0].Content, 1)
	assert.Equal(t, "output_text", out[0].Content[0].Type)
	assert.Equal(t, "hello world", out[0].Content[0].Text)
}

// TestRenderChatResponseAsResponseAPI_EmptyCompletionEmitsOutputArray asserts the wire
// JSON for an empty completion contains "output":[] — never "output":null. This is the
// regression test for strict OpenAI Python/TS SDK clients (Pydantic / Zod) rejecting null
// for list[OutputItem].
func TestRenderChatResponseAsResponseAPI_EmptyCompletionEmitsOutputArray(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		textResp *openai_compatible.SlimTextResponse
	}{
		{
			name:     "no choices",
			textResp: &openai_compatible.SlimTextResponse{},
		},
		{
			name: "single choice with empty message and no tool_calls",
			textResp: &openai_compatible.SlimTextResponse{
				Choices: []openai_compatible.TextResponseChoice{
					{Message: relaymodel.Message{Role: "assistant", Content: ""}, FinishReason: "stop"},
				},
			},
		},
		{
			name: "content_filter refusal",
			textResp: &openai_compatible.SlimTextResponse{
				Choices: []openai_compatible.TextResponseChoice{
					{Message: relaymodel.Message{Role: "assistant", Content: ""}, FinishReason: "content_filter"},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			req := &openai.ResponseAPIRequest{Model: "gpt-4o-mini"}
			meta := &metalib.Meta{ActualModelName: "gpt-4o-mini"}
			require.NoError(t, renderChatResponseAsResponseAPI(c, http.StatusOK, tc.textResp, req, meta))

			body := w.Body.Bytes()
			assert.Contains(t, string(body), `"output":[]`, "wire JSON must contain output:[] for strict SDK compatibility")
			assert.False(t, bytes.Contains(body, []byte(`"output":null`)), "wire JSON must NEVER contain output:null")

			// Also verify it parses back as an array (not nil) when decoded with a sentinel.
			var parsed map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &parsed))
			require.Contains(t, parsed, "output")
			outputRaw := bytes.TrimSpace(parsed["output"])
			assert.Equal(t, "[]", string(outputRaw), "output field must serialize as an empty JSON array")
		})
	}
}

// TestRenderChatResponseAsResponseAPI_PopulatedRoundTripsThroughOpenAISDKShape ensures a
// normal completion still serializes with a populated output array of the expected shape.
func TestRenderChatResponseAsResponseAPI_PopulatedRoundTripsThroughOpenAISDKShape(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	textResp := &openai_compatible.SlimTextResponse{
		Choices: []openai_compatible.TextResponseChoice{
			{
				Message:      relaymodel.Message{Role: "assistant", Content: "hi there"},
				FinishReason: "stop",
			},
		},
	}
	req := &openai.ResponseAPIRequest{Model: "gpt-4o-mini"}
	meta := &metalib.Meta{ActualModelName: "gpt-4o-mini"}
	require.NoError(t, renderChatResponseAsResponseAPI(c, http.StatusOK, textResp, req, meta))

	body := w.Body.Bytes()
	assert.False(t, bytes.Contains(body, []byte(`"output":null`)))

	var resp openai.ResponseAPIResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.Output)
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "message", resp.Output[0].Type)
	assert.Equal(t, "assistant", resp.Output[0].Role)
	require.Len(t, resp.Output[0].Content, 1)
	assert.Equal(t, "hi there", resp.Output[0].Content[0].Text)
}
