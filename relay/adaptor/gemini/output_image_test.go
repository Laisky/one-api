package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/model"
)

// TestCountGeminiOutputImages verifies inline and file image parts are counted correctly.
func TestCountGeminiOutputImages(t *testing.T) {
	response := &ChatResponse{
		Candidates: []ChatCandidate{
			{
				Content: ChatContent{Parts: []Part{
					{Text: "hello"},
					{InlineData: &InlineData{MimeType: "image/png", Data: "abc"}},
				}},
			},
			{
				Content: ChatContent{Parts: []Part{
					{InlineData: &InlineData{MimeType: "image/jpeg", Data: "def"}},
					{FileData: &FileData{MimeType: "image/png", FileURI: "gs://bucket/image.png"}},
				}},
			},
		},
	}

	counts := countGeminiOutputImages(response)
	require.Equal(t, 3, counts.Total)
	require.Equal(t, 2, counts.Inline)
	require.Equal(t, 1, counts.File)
}

// TestRecordGeminiOutputImageCount verifies output image counts are aggregated in context.
func TestRecordGeminiOutputImageCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	recordGeminiOutputImageCount(c, 2)
	recordGeminiOutputImageCount(c, 1)

	raw, ok := c.Get(ctxkey.OutputImageCount)
	require.True(t, ok, "expected output image count in context")
	require.Equal(t, 3, raw.(int))
}

// TestResponseGeminiChat2OpenAI_FileData verifies file-based images are exposed as image_url content.
func TestResponseGeminiChat2OpenAI_FileData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	response := &ChatResponse{
		Candidates: []ChatCandidate{
			{
				Content: ChatContent{
					Parts: []Part{
						{FileData: &FileData{MimeType: "image/png", FileURI: "gs://bucket/image.png"}},
					},
				},
			},
		},
	}

	result := responseGeminiChat2OpenAI(c, response)
	require.NotNil(t, result)
	require.Len(t, result.Choices, 1)

	content, ok := result.Choices[0].Message.Content.([]model.MessageContent)
	require.True(t, ok, "expected structured content for image output")
	require.Len(t, content, 1)
	require.NotNil(t, content[0].ImageURL)
	require.Equal(t, "gs://bucket/image.png", content[0].ImageURL.Url)
}
