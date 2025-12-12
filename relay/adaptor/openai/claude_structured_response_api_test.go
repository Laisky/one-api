package openai

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// TestConvertClaudeRequest_PromotedStructuredForcesResponseAPI ensures that when a Claude structured
// request is promoted to OpenAI json_schema, OpenAI/Azure channels route it through the Response API
// surface and preserve the schema in text.format.
func TestConvertClaudeRequest_PromotedStructuredForcesResponseAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic":      map[string]any{"type": "string"},
			"confidence": map[string]any{"type": "number"},
		},
		"required":             []any{"topic", "confidence"},
		"additionalProperties": false,
	}

	req := &model.ClaudeRequest{
		Model:     "gpt-5-mini",
		MaxTokens: 256,
		Messages: []model.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Provide structured topic and confidence JSON."},
				},
			},
		},
		Tools: []model.ClaudeTool{
			{
				Name:        "topic_classifier",
				Description: "Return structured topic and confidence data",
				InputSchema: schema,
			},
		},
		ToolChoice: map[string]any{"type": "tool", "name": "topic_classifier"},
	}

	t.Run("openai", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-5-mini", Mode: relaymode.ClaudeMessages})

		adaptor := &Adaptor{}
		converted, err := adaptor.ConvertClaudeRequest(c, req)
		require.NoError(t, err)

		respReq, ok := converted.(*ResponseAPIRequest)
		require.True(t, ok, "expected ResponseAPIRequest, got %T", converted)
		require.NotNil(t, respReq.Text)
		require.NotNil(t, respReq.Text.Format)
		assert.Equal(t, "json_schema", respReq.Text.Format.Type)
		assert.Equal(t, "topic_classifier", respReq.Text.Format.Name)
		assert.Equal(t, schema, respReq.Text.Format.Schema)
	})

	t.Run("azure", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.Azure, ActualModelName: "gpt-5-nano", Mode: relaymode.ClaudeMessages})

		reqAzure := *req
		reqAzure.Model = "gpt-5-nano"

		adaptor := &Adaptor{}
		converted, err := adaptor.ConvertClaudeRequest(c, &reqAzure)
		require.NoError(t, err)

		respReq, ok := converted.(*ResponseAPIRequest)
		require.True(t, ok, "expected ResponseAPIRequest, got %T", converted)
		require.NotNil(t, respReq.Text)
		require.NotNil(t, respReq.Text.Format)
		assert.Equal(t, "json_schema", respReq.Text.Format.Type)
		assert.Equal(t, "topic_classifier", respReq.Text.Format.Name)
		assert.Equal(t, schema, respReq.Text.Format.Schema)
	})
}
