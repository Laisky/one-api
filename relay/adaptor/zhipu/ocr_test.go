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

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// ---------------------------------------------------------------------------
// Unit tests for OCRRequest / OCRResponse model serialization
// ---------------------------------------------------------------------------

func TestOCRRequest_JSONSerialization(t *testing.T) {
	t.Parallel()

	t.Run("minimal required fields", func(t *testing.T) {
		req := OCRRequest{
			Model: "glm-ocr",
			File:  "https://example.com/test.png",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Equal(t, "glm-ocr", m["model"])
		assert.Equal(t, "https://example.com/test.png", m["file"])
		// optional fields must be omitted
		assert.NotContains(t, m, "request_id")
		assert.NotContains(t, m, "user_id")
		assert.NotContains(t, m, "return_crop_images")
		assert.NotContains(t, m, "need_layout_visualization")
		assert.NotContains(t, m, "start_page_id")
		assert.NotContains(t, m, "end_page_id")
	})

	t.Run("all optional fields", func(t *testing.T) {
		boolTrue := true
		boolFalse := false
		startPage := 1
		endPage := 5
		req := OCRRequest{
			Model:                   "glm-ocr",
			File:                    "https://example.com/doc.pdf",
			RequestID:               "req-123",
			UserID:                  "user-456",
			ReturnCropImages:        &boolTrue,
			NeedLayoutVisualization: &boolFalse,
			StartPageID:             &startPage,
			EndPageID:               &endPage,
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Equal(t, "req-123", m["request_id"])
		assert.Equal(t, "user-456", m["user_id"])
		assert.Equal(t, true, m["return_crop_images"])
		assert.Equal(t, false, m["need_layout_visualization"])
		assert.Equal(t, float64(1), m["start_page_id"])
		assert.Equal(t, float64(5), m["end_page_id"])
	})
}

func TestOCRResponse_JSONDeserialization(t *testing.T) {
	t.Parallel()

	t.Run("full response from API", func(t *testing.T) {
		raw := `{
			"id": "task-abc123",
			"created": 1700000000,
			"model": "glm-ocr",
			"md_results": "# Title\n\nSome parsed text",
			"request_id": "req-xyz",
			"usage": {
				"prompt_tokens": 100,
				"completion_tokens": 200,
				"total_tokens": 300,
				"prompt_tokens_details": {
					"cached_tokens": 10
				}
			}
		}`
		var resp OCRResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &resp))

		assert.Equal(t, "task-abc123", resp.ID)
		assert.Equal(t, int64(1700000000), resp.Created)
		assert.Equal(t, "glm-ocr", resp.Model)
		assert.Equal(t, "# Title\n\nSome parsed text", resp.MdResults)
		assert.Equal(t, "req-xyz", resp.RequestID)
		assert.Equal(t, 100, resp.Usage.PromptTokens)
		assert.Equal(t, 200, resp.Usage.CompletionTokens)
		assert.Equal(t, 300, resp.Usage.TotalTokens)
		require.NotNil(t, resp.Usage.PromptTokensDetails)
		assert.Equal(t, 10, resp.Usage.PromptTokensDetails.CachedTokens)
	})

	t.Run("minimal response without optional fields", func(t *testing.T) {
		raw := `{
			"md_results": "Hello",
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15
			}
		}`
		var resp OCRResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &resp))

		assert.Equal(t, "Hello", resp.MdResults)
		assert.Equal(t, "", resp.ID)
		assert.Equal(t, int64(0), resp.Created)
		assert.Equal(t, 15, resp.Usage.TotalTokens)
		assert.Nil(t, resp.Usage.PromptTokensDetails)
	})

	t.Run("old content field is not deserialized", func(t *testing.T) {
		// Ensure the old "content" field does NOT populate MdResults
		raw := `{"content": "should be ignored", "md_results": "correct value", "usage": {}}`
		var resp OCRResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &resp))
		assert.Equal(t, "correct value", resp.MdResults)
	})
}

// ---------------------------------------------------------------------------
// Unit tests for isOCRModel
// ---------------------------------------------------------------------------

func TestIsOCRModel(t *testing.T) {
	t.Parallel()
	assert.True(t, isOCRModel("glm-ocr"))
	assert.False(t, isOCRModel("glm-4"))
	assert.False(t, isOCRModel("glm-ocr-v2"))
	assert.False(t, isOCRModel(""))
}

// ---------------------------------------------------------------------------
// Unit tests for ConvertOCRRequest
// ---------------------------------------------------------------------------

func TestConvertOCRRequest(t *testing.T) {
	t.Parallel()

	t.Run("extracts image_url from user message", func(t *testing.T) {
		imgURL := "https://example.com/image.png"
		req := model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			Messages: []model.Message{
				{
					Role: "user",
					Content: []any{
						map[string]any{
							"type": "image_url",
							"image_url": map[string]any{
								"url": imgURL,
							},
						},
					},
				},
			},
		}

		ocrReq, err := ConvertOCRRequest(req)
		require.NoError(t, err)
		assert.Equal(t, "glm-ocr", ocrReq.Model)
		assert.Equal(t, imgURL, ocrReq.File)
	})

	t.Run("returns error when no image_url present", func(t *testing.T) {
		req := model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			Messages: []model.Message{
				{
					Role:    "user",
					Content: "just text, no image",
				},
			},
		}

		_, err := ConvertOCRRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "image_url")
	})

	t.Run("skips non-user messages", func(t *testing.T) {
		req := model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			Messages: []model.Message{
				{
					Role: "system",
					Content: []any{
						map[string]any{
							"type": "image_url",
							"image_url": map[string]any{
								"url": "https://example.com/system.png",
							},
						},
					},
				},
			},
		}

		_, err := ConvertOCRRequest(req)
		require.Error(t, err)
	})

	t.Run("forwards user_id from User field", func(t *testing.T) {
		req := model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			User:  "user-abc",
			Messages: []model.Message{
				{
					Role: "user",
					Content: []any{
						map[string]any{
							"type":      "image_url",
							"image_url": map[string]any{"url": "https://example.com/img.png"},
						},
					},
				},
			},
		}

		ocrReq, err := ConvertOCRRequest(req)
		require.NoError(t, err)
		assert.Equal(t, "user-abc", ocrReq.UserID)
	})

	t.Run("forwards extra_body parameters", func(t *testing.T) {
		req := model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			ExtraBody: map[string]any{
				"request_id":                "req-999",
				"return_crop_images":        true,
				"need_layout_visualization": true,
				"start_page_id":             float64(2),
				"end_page_id":               float64(10),
			},
			Messages: []model.Message{
				{
					Role: "user",
					Content: []any{
						map[string]any{
							"type":      "image_url",
							"image_url": map[string]any{"url": "https://example.com/doc.pdf"},
						},
					},
				},
			},
		}

		ocrReq, err := ConvertOCRRequest(req)
		require.NoError(t, err)
		assert.Equal(t, "req-999", ocrReq.RequestID)
		require.NotNil(t, ocrReq.ReturnCropImages)
		assert.True(t, *ocrReq.ReturnCropImages)
		require.NotNil(t, ocrReq.NeedLayoutVisualization)
		assert.True(t, *ocrReq.NeedLayoutVisualization)
		require.NotNil(t, ocrReq.StartPageID)
		assert.Equal(t, 2, *ocrReq.StartPageID)
		require.NotNil(t, ocrReq.EndPageID)
		assert.Equal(t, 10, *ocrReq.EndPageID)
	})

	t.Run("extra_body with no matching keys is fine", func(t *testing.T) {
		req := model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			ExtraBody: map[string]any{
				"unknown_param": "value",
			},
			Messages: []model.Message{
				{
					Role: "user",
					Content: []any{
						map[string]any{
							"type":      "image_url",
							"image_url": map[string]any{"url": "https://example.com/img.png"},
						},
					},
				},
			},
		}

		ocrReq, err := ConvertOCRRequest(req)
		require.NoError(t, err)
		assert.Equal(t, "", ocrReq.RequestID)
		assert.Nil(t, ocrReq.ReturnCropImages)
	})
}

// ---------------------------------------------------------------------------
// Behavioral tests for OCRHandler (native response passthrough)
// ---------------------------------------------------------------------------

func newTestGinContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func makeHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestOCRHandler(t *testing.T) {
	t.Parallel()

	t.Run("native response is passed through with all fields", func(t *testing.T) {
		apiResp := `{
			"id": "task-ocr-001",
			"created": 1700000000,
			"model": "glm-ocr",
			"md_results": "# Invoice\n\nTotal: $100.00",
			"layout_details": [[{"index": 1, "label": "text", "content": "Invoice"}]],
			"layout_visualization": ["https://example.com/vis.png"],
			"data_info": {"num_pages": 1, "pages": [{"width": 600, "height": 800}]},
			"request_id": "req-001",
			"usage": {
				"prompt_tokens": 50,
				"completion_tokens": 120,
				"total_tokens": 170,
				"prompt_tokens_details": {
					"cached_tokens": 5
				}
			}
		}`

		c, w := newTestGinContext()
		resp := makeHTTPResponse(http.StatusOK, apiResp)

		errResp, usage := OCRHandler(c, resp, "glm-ocr")
		require.Nil(t, errResp)
		require.NotNil(t, usage)

		// Verify usage extracted for billing
		assert.Equal(t, 50, usage.PromptTokens)
		assert.Equal(t, 120, usage.CompletionTokens)
		assert.Equal(t, 170, usage.TotalTokens)

		// Verify response body is the native format (not chat completion)
		var nativeResp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nativeResp))

		assert.Equal(t, "task-ocr-001", nativeResp["id"])
		assert.Equal(t, float64(1700000000), nativeResp["created"])
		assert.Equal(t, "glm-ocr", nativeResp["model"])
		assert.Equal(t, "# Invoice\n\nTotal: $100.00", nativeResp["md_results"])
		assert.Equal(t, "req-001", nativeResp["request_id"])

		// Native fields must be preserved
		assert.NotNil(t, nativeResp["layout_details"], "layout_details should be passed through")
		assert.NotNil(t, nativeResp["layout_visualization"], "layout_visualization should be passed through")
		assert.NotNil(t, nativeResp["data_info"], "data_info should be passed through")

		// Must NOT have chat completion fields
		assert.Nil(t, nativeResp["object"], "should not have chat.completion object field")
		assert.Nil(t, nativeResp["choices"], "should not have choices field")

		// Verify usage in response JSON
		usageJSON := nativeResp["usage"].(map[string]any)
		assert.Equal(t, float64(50), usageJSON["prompt_tokens"])
		assert.Equal(t, float64(120), usageJSON["completion_tokens"])
		assert.Equal(t, float64(170), usageJSON["total_tokens"])
		details := usageJSON["prompt_tokens_details"].(map[string]any)
		assert.Equal(t, float64(5), details["cached_tokens"])
	})

	t.Run("minimal response without optional fields", func(t *testing.T) {
		apiResp := `{
			"md_results": "parsed text",
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20,
				"total_tokens": 30
			}
		}`

		c, w := newTestGinContext()
		resp := makeHTTPResponse(http.StatusOK, apiResp)

		errResp, usage := OCRHandler(c, resp, "glm-ocr")
		require.Nil(t, errResp)
		require.NotNil(t, usage)
		assert.Equal(t, 30, usage.TotalTokens)

		var nativeResp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nativeResp))

		assert.Equal(t, "parsed text", nativeResp["md_results"])
	})

	t.Run("empty md_results is passed through", func(t *testing.T) {
		apiResp := `{
			"id": "task-empty",
			"created": 1700000000,
			"model": "glm-ocr",
			"md_results": "",
			"usage": {"total_tokens": 5}
		}`

		c, w := newTestGinContext()
		resp := makeHTTPResponse(http.StatusOK, apiResp)

		errResp, _ := OCRHandler(c, resp, "glm-ocr")
		require.Nil(t, errResp)

		var nativeResp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &nativeResp))
		assert.Equal(t, "", nativeResp["md_results"])
	})

	t.Run("invalid JSON response returns error", func(t *testing.T) {
		c, _ := newTestGinContext()
		resp := makeHTTPResponse(http.StatusOK, "not json")

		errResp, usage := OCRHandler(c, resp, "glm-ocr")
		require.NotNil(t, errResp)
		assert.Nil(t, usage)
		assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	})

	t.Run("HTTP status code is forwarded", func(t *testing.T) {
		apiResp := `{"md_results": "text", "usage": {"total_tokens": 5}}`

		c, w := newTestGinContext()
		resp := makeHTTPResponse(http.StatusOK, apiResp)

		errResp, _ := OCRHandler(c, resp, "glm-ocr")
		require.Nil(t, errResp)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ---------------------------------------------------------------------------
// Integration-style test: Adaptor.ConvertRequest routes to OCR
// ---------------------------------------------------------------------------

func TestAdaptorConvertRequest_OCRRouting(t *testing.T) {
	t.Parallel()

	t.Run("glm-ocr model routes to ConvertOCRRequest", func(t *testing.T) {
		adaptor := &Adaptor{}
		c := newZhipuContext()

		req := &model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			Messages: []model.Message{
				{
					Role: "user",
					Content: []any{
						map[string]any{
							"type":      "image_url",
							"image_url": map[string]any{"url": "https://example.com/img.png"},
						},
					},
				},
			},
		}

		result, err := adaptor.ConvertRequest(c, 0, req)
		require.NoError(t, err)

		ocrReq, ok := result.(*OCRRequest)
		require.True(t, ok, "expected *OCRRequest, got %T", result)
		assert.Equal(t, "glm-ocr", ocrReq.Model)
		assert.Equal(t, "https://example.com/img.png", ocrReq.File)
	})

	t.Run("glm-ocr without image returns error", func(t *testing.T) {
		adaptor := &Adaptor{}
		c := newZhipuContext()

		req := &model.GeneralOpenAIRequest{
			Model: "glm-ocr",
			Messages: []model.Message{
				{Role: "user", Content: "no image here"},
			},
		}

		_, err := adaptor.ConvertRequest(c, 0, req)
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Test: GetRequestURL for OCR model
// ---------------------------------------------------------------------------

func TestGetRequestURL_OCR(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	meta := &meta.Meta{
		BaseURL:         "https://open.bigmodel.cn",
		ActualModelName: "glm-ocr",
	}

	url, err := adaptor.GetRequestURL(meta)
	require.NoError(t, err)
	assert.Equal(t, "https://open.bigmodel.cn/api/paas/v4/layout_parsing", url)
}
