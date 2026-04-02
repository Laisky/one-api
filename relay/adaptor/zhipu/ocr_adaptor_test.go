package zhipu

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// ---------------------------------------------------------------------------
// Verify Adaptor satisfies OCRAdaptor interface
// ---------------------------------------------------------------------------

var _ adaptor.OCRAdaptor = (*Adaptor)(nil)

// ---------------------------------------------------------------------------
// GetRequestURL with relaymode.OCR
// ---------------------------------------------------------------------------

func TestGetRequestURL_OCRMode(t *testing.T) {
	t.Parallel()

	t.Run("OCR mode returns layout_parsing URL", func(t *testing.T) {
		a := &Adaptor{}
		m := &meta.Meta{
			Mode:            relaymode.OCR,
			BaseURL:         "https://open.bigmodel.cn",
			ActualModelName: "glm-ocr",
		}
		url, err := a.GetRequestURL(m)
		require.NoError(t, err)
		assert.Equal(t, "https://open.bigmodel.cn/api/paas/v4/layout_parsing", url)
	})

	t.Run("OCR mode with custom base URL preserves scheme and host", func(t *testing.T) {
		a := &Adaptor{}
		m := &meta.Meta{
			Mode:            relaymode.OCR,
			BaseURL:         "http://localhost:8080",
			ActualModelName: "glm-ocr",
		}
		url, err := a.GetRequestURL(m)
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:8080/api/paas/v4/layout_parsing", url)
	})

	t.Run("OCR detected by model name still works (backward compat)", func(t *testing.T) {
		a := &Adaptor{}
		m := &meta.Meta{
			Mode:            relaymode.ChatCompletions, // not OCR mode, but model is glm-ocr
			BaseURL:         "https://open.bigmodel.cn",
			ActualModelName: "glm-ocr",
		}
		url, err := a.GetRequestURL(m)
		require.NoError(t, err)
		assert.Equal(t, "https://open.bigmodel.cn/api/paas/v4/layout_parsing", url)
	})
}

// ---------------------------------------------------------------------------
// ConvertOCRRequest (adaptor.OCRAdaptor interface)
// ---------------------------------------------------------------------------

func TestAdaptorConvertOCRRequest(t *testing.T) {
	t.Parallel()

	t.Run("minimal required fields", func(t *testing.T) {
		a := &Adaptor{}
		c := newZhipuContext()

		req := &model.OCRRequest{
			Model: "glm-ocr",
			File:  "https://example.com/test.pdf",
		}

		result, err := a.ConvertOCRRequest(c, req)
		require.NoError(t, err)

		zhipuReq, ok := result.(*OCRRequest)
		require.True(t, ok, "expected *OCRRequest, got %T", result)
		assert.Equal(t, "glm-ocr", zhipuReq.Model)
		assert.Equal(t, "https://example.com/test.pdf", zhipuReq.File)
		assert.Empty(t, zhipuReq.RequestID)
		assert.Empty(t, zhipuReq.UserID)
		assert.Nil(t, zhipuReq.ReturnCropImages)
		assert.Nil(t, zhipuReq.NeedLayoutVisualization)
		assert.Nil(t, zhipuReq.StartPageID)
		assert.Nil(t, zhipuReq.EndPageID)
	})

	t.Run("all optional fields forwarded", func(t *testing.T) {
		a := &Adaptor{}
		c := newZhipuContext()

		boolTrue := true
		boolFalse := false
		startPage := 2
		endPage := 10

		req := &model.OCRRequest{
			Model:                   "glm-ocr",
			File:                    "https://example.com/doc.pdf",
			RequestID:               "req-abc",
			UserID:                  "user-xyz",
			ReturnCropImages:        &boolTrue,
			NeedLayoutVisualization: &boolFalse,
			StartPageID:             &startPage,
			EndPageID:               &endPage,
		}

		result, err := a.ConvertOCRRequest(c, req)
		require.NoError(t, err)

		zhipuReq := result.(*OCRRequest)
		assert.Equal(t, "req-abc", zhipuReq.RequestID)
		assert.Equal(t, "user-xyz", zhipuReq.UserID)
		require.NotNil(t, zhipuReq.ReturnCropImages)
		assert.True(t, *zhipuReq.ReturnCropImages)
		require.NotNil(t, zhipuReq.NeedLayoutVisualization)
		assert.False(t, *zhipuReq.NeedLayoutVisualization)
		require.NotNil(t, zhipuReq.StartPageID)
		assert.Equal(t, 2, *zhipuReq.StartPageID)
		require.NotNil(t, zhipuReq.EndPageID)
		assert.Equal(t, 10, *zhipuReq.EndPageID)
	})

	t.Run("nil request returns error", func(t *testing.T) {
		a := &Adaptor{}
		c := newZhipuContext()

		_, err := a.ConvertOCRRequest(c, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("converted result serializes to correct JSON", func(t *testing.T) {
		a := &Adaptor{}
		c := newZhipuContext()

		startPage := 1
		req := &model.OCRRequest{
			Model:       "glm-ocr",
			File:        "https://example.com/img.png",
			StartPageID: &startPage,
		}

		result, err := a.ConvertOCRRequest(c, req)
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Equal(t, "glm-ocr", m["model"])
		assert.Equal(t, "https://example.com/img.png", m["file"])
		assert.Equal(t, float64(1), m["start_page_id"])
		// Omitted optional fields should not appear
		assert.NotContains(t, m, "request_id")
		assert.NotContains(t, m, "user_id")
		assert.NotContains(t, m, "return_crop_images")
		assert.NotContains(t, m, "need_layout_visualization")
		assert.NotContains(t, m, "end_page_id")
	})
}

// ---------------------------------------------------------------------------
// DoOCRResponse (adaptor.OCRAdaptor interface)
// ---------------------------------------------------------------------------

func TestAdaptorDoOCRResponse(t *testing.T) {
	t.Parallel()

	newOCRContext := func() (*gin.Context, *httptest.ResponseRecorder) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		return c, w
	}

	t.Run("successful response returns usage and passes through native format", func(t *testing.T) {
		a := &Adaptor{}
		c, w := newOCRContext()

		apiResp := `{
			"id": "task-001",
			"created": 1700000000,
			"model": "glm-ocr",
			"md_results": "# Heading\n\nParagraph text",
			"layout_details": [[{"index": 1, "label": "text"}]],
			"usage": {
				"prompt_tokens": 80,
				"completion_tokens": 150,
				"total_tokens": 230
			}
		}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(apiResp)),
			Header:     make(http.Header),
		}

		m := &meta.Meta{ActualModelName: "glm-ocr"}
		usage, errResp := a.DoOCRResponse(c, resp, m)
		require.Nil(t, errResp)
		require.NotNil(t, usage)
		assert.Equal(t, 80, usage.PromptTokens)
		assert.Equal(t, 150, usage.CompletionTokens)
		assert.Equal(t, 230, usage.TotalTokens)

		// Verify client response is native Zhipu format (not chat completion)
		var nativeResp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nativeResp))
		assert.Equal(t, "task-001", nativeResp["id"])
		assert.Equal(t, "# Heading\n\nParagraph text", nativeResp["md_results"])
		assert.NotNil(t, nativeResp["layout_details"])
		assert.Nil(t, nativeResp["object"], "should not have chat.completion object")
		assert.Nil(t, nativeResp["choices"], "should not have choices")
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		a := &Adaptor{}
		c, _ := newOCRContext()

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("not json")),
			Header:     make(http.Header),
		}

		m := &meta.Meta{ActualModelName: "glm-ocr"}
		usage, errResp := a.DoOCRResponse(c, resp, m)
		require.NotNil(t, errResp)
		assert.Nil(t, usage)
		assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	})
}
