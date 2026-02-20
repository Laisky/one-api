package deepseek

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestConvertRequest_NormalizesToolArrayContentToString(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{Role: "user", Content: "hello"},
			{
				Role:       "tool",
				ToolCallId: "call_1",
				Content: []any{
					map[string]any{"type": "text", "text": "README.md\n"},
				},
			},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "README.md\n", converted.Messages[1].Content)
}

func TestConvertRequest_NormalizesToolMapContentByJSONFallback(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{
				Role:       "tool",
				ToolCallId: "call_2",
				Content: map[string]any{
					"stdout":    "ok",
					"exit_code": 0,
				},
			},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)

	contentStr, ok := converted.Messages[0].Content.(string)
	require.True(t, ok)
	require.Contains(t, contentStr, `"stdout":"ok"`)
	require.Contains(t, contentStr, `"exit_code":0`)
}

func TestConvertRequest_NormalizesNilToolContentToEmptyString(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{Role: "tool", ToolCallId: "call_3", Content: nil},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "", converted.Messages[0].Content)
}

func TestConvertRequest_DoesNotChangeNonToolArrayContent(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	originalContent := []any{map[string]any{"type": "text", "text": "hello"}}
	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []model.Message{
			{Role: "user", Content: originalContent},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, originalContent, converted.Messages[0].Content)
}
