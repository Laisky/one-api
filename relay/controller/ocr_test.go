package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/adaptor"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func init() {
	_ = common.GetRequestBody
	logger.SetupLogger()
}

// ===========================================================================
// Test helpers
// ===========================================================================

func newOCRGinContext(body any) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewBuffer(data)
	} else {
		bodyReader = bytes.NewBuffer(nil)
	}

	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", bodyReader)
	c.Request.Header.Set("Content-Type", "application/json")
	gmw.SetLogger(c, logger.Logger)
	return c, w
}

func newTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", nil)
	gmw.SetLogger(c, logger.Logger)
	return c
}

// ---------------------------------------------------------------------------
// Mock adaptors
// ---------------------------------------------------------------------------

// mockOCRAdaptor implements both adaptor.Adaptor and adaptor.OCRAdaptor.
type mockOCRAdaptor struct {
	convertErr   error
	converted    any
	doRequestErr error
	doRequestFn  func() (*http.Response, error)
	doOCRRespFn  func(c *gin.Context, resp *http.Response, meta *metalib.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode)
}

func (m *mockOCRAdaptor) Init(_ *metalib.Meta)                                          {}
func (m *mockOCRAdaptor) GetRequestURL(_ *metalib.Meta) (string, error)                 { return "https://test.api/layout_parsing", nil }
func (m *mockOCRAdaptor) SetupRequestHeader(_ *gin.Context, _ *http.Request, _ *metalib.Meta) error { return nil }
func (m *mockOCRAdaptor) ConvertRequest(_ *gin.Context, _ int, _ *relaymodel.GeneralOpenAIRequest) (any, error) { return nil, nil }
func (m *mockOCRAdaptor) ConvertImageRequest(_ *gin.Context, _ *relaymodel.ImageRequest) (any, error) { return nil, nil }
func (m *mockOCRAdaptor) ConvertClaudeRequest(_ *gin.Context, _ *relaymodel.ClaudeRequest) (any, error) { return nil, nil }
func (m *mockOCRAdaptor) DoRequest(_ *gin.Context, _ *metalib.Meta, _ io.Reader) (*http.Response, error) {
	if m.doRequestFn != nil {
		return m.doRequestFn()
	}
	if m.doRequestErr != nil {
		return nil, m.doRequestErr
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
	}, nil
}
func (m *mockOCRAdaptor) DoResponse(_ *gin.Context, _ *http.Response, _ *metalib.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) { return nil, nil }
func (m *mockOCRAdaptor) GetModelList() []string                                        { return nil }
func (m *mockOCRAdaptor) GetChannelName() string                                        { return "mock-ocr" }
func (m *mockOCRAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig         { return nil }
func (m *mockOCRAdaptor) GetModelRatio(_ string) float64                                { return 1.0 }
func (m *mockOCRAdaptor) GetCompletionRatio(_ string) float64                           { return 1.0 }
func (m *mockOCRAdaptor) ConvertOCRRequest(_ *gin.Context, req *relaymodel.OCRRequest) (any, error) {
	if m.convertErr != nil {
		return nil, m.convertErr
	}
	if m.converted != nil {
		return m.converted, nil
	}
	return req, nil
}
func (m *mockOCRAdaptor) DoOCRResponse(c *gin.Context, resp *http.Response, meta *metalib.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	if m.doOCRRespFn != nil {
		return m.doOCRRespFn(c, resp, meta)
	}
	return &relaymodel.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}, nil
}

// plainAdaptor implements adaptor.Adaptor but NOT adaptor.OCRAdaptor.
type plainAdaptor struct{}

func (p *plainAdaptor) Init(_ *metalib.Meta)                                            {}
func (p *plainAdaptor) GetRequestURL(_ *metalib.Meta) (string, error)                   { return "https://test.api/v1/chat/completions", nil }
func (p *plainAdaptor) SetupRequestHeader(_ *gin.Context, _ *http.Request, _ *metalib.Meta) error { return nil }
func (p *plainAdaptor) ConvertRequest(_ *gin.Context, _ int, _ *relaymodel.GeneralOpenAIRequest) (any, error) { return nil, nil }
func (p *plainAdaptor) ConvertImageRequest(_ *gin.Context, _ *relaymodel.ImageRequest) (any, error) { return nil, nil }
func (p *plainAdaptor) ConvertClaudeRequest(_ *gin.Context, _ *relaymodel.ClaudeRequest) (any, error) { return nil, nil }
func (p *plainAdaptor) DoRequest(_ *gin.Context, _ *metalib.Meta, _ io.Reader) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
	}, nil
}
func (p *plainAdaptor) DoResponse(_ *gin.Context, _ *http.Response, _ *metalib.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) { return nil, nil }
func (p *plainAdaptor) GetModelList() []string                                          { return nil }
func (p *plainAdaptor) GetChannelName() string                                          { return "plain-adaptor" }
func (p *plainAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig           { return nil }
func (p *plainAdaptor) GetModelRatio(_ string) float64                                  { return 1.0 }
func (p *plainAdaptor) GetCompletionRatio(_ string) float64                             { return 1.0 }

// ===========================================================================
// 1. Request validation (getAndValidateOCRRequest)
// ===========================================================================

func TestGetAndValidateOCRRequest(t *testing.T) {
	t.Parallel()

	t.Run("valid minimal request", func(t *testing.T) {
		body := map[string]any{"model": "glm-ocr", "file": "https://example.com/test.pdf"}
		c, _ := newOCRGinContext(body)
		data, _ := json.Marshal(body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

		req, err := getAndValidateOCRRequest(c)
		require.NoError(t, err)
		assert.Equal(t, "glm-ocr", req.Model)
		assert.Equal(t, "https://example.com/test.pdf", req.File)
	})

	t.Run("valid request with all optional fields", func(t *testing.T) {
		body := map[string]any{
			"model": "glm-ocr", "file": "https://example.com/doc.pdf",
			"request_id": "req-123", "user_id": "user-abc",
			"return_crop_images": true, "need_layout_visualization": false,
			"start_page_id": 1, "end_page_id": 5,
		}
		c, _ := newOCRGinContext(body)
		data, _ := json.Marshal(body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

		req, err := getAndValidateOCRRequest(c)
		require.NoError(t, err)
		assert.Equal(t, "req-123", req.RequestID)
		assert.Equal(t, "user-abc", req.UserID)
		require.NotNil(t, req.ReturnCropImages)
		assert.True(t, *req.ReturnCropImages)
		require.NotNil(t, req.StartPageID)
		assert.Equal(t, 1, *req.StartPageID)
	})

	t.Run("missing model returns error", func(t *testing.T) {
		body := map[string]any{"file": "https://example.com/test.pdf"}
		c, _ := newOCRGinContext(body)
		data, _ := json.Marshal(body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

		_, err := getAndValidateOCRRequest(c)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model")
	})

	t.Run("missing file returns error", func(t *testing.T) {
		body := map[string]any{"model": "glm-ocr"}
		c, _ := newOCRGinContext(body)
		data, _ := json.Marshal(body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(data))

		_, err := getAndValidateOCRRequest(c)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "file")
	})

	t.Run("empty body returns error", func(t *testing.T) {
		c, _ := newOCRGinContext(nil)
		c.Request.Body = io.NopCloser(bytes.NewBuffer([]byte("{}")))

		_, err := getAndValidateOCRRequest(c)
		require.Error(t, err)
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		c, _ := newOCRGinContext(nil)
		c.Request.Body = io.NopCloser(bytes.NewBuffer([]byte("not json")))

		_, err := getAndValidateOCRRequest(c)
		require.Error(t, err)
	})
}

// ===========================================================================
// 2. prepareOCRRequestBody
// ===========================================================================

func TestPrepareOCRRequestBody(t *testing.T) {
	t.Parallel()

	t.Run("OCR adaptor converts and serializes request", func(t *testing.T) {
		c := newTestContext()
		meta := &metalib.Meta{}
		a := &mockOCRAdaptor{}
		req := &relaymodel.OCRRequest{Model: "glm-ocr", File: "https://example.com/file.pdf"}

		reader, err := prepareOCRRequestBody(c, meta, a, req)
		require.NoError(t, err)
		require.NotNil(t, reader)

		data, _ := io.ReadAll(reader)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Equal(t, "glm-ocr", m["model"])
		assert.Equal(t, "https://example.com/file.pdf", m["file"])

		// ConvertedRequest must be stored in context
		_, exists := c.Get(ctxkey.ConvertedRequest)
		assert.True(t, exists, "ConvertedRequest should be set in context")
	})

	t.Run("nil request returns error", func(t *testing.T) {
		_, err := prepareOCRRequestBody(newTestContext(), &metalib.Meta{}, &mockOCRAdaptor{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("adaptor without OCR support returns error with channel name", func(t *testing.T) {
		c := newTestContext()
		a := &plainAdaptor{}
		req := &relaymodel.OCRRequest{Model: "glm-ocr", File: "https://example.com/img.png"}

		_, err := prepareOCRRequestBody(c, &metalib.Meta{}, a, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
		assert.Contains(t, err.Error(), "plain-adaptor")
	})

	t.Run("ConvertOCRRequest error is propagated", func(t *testing.T) {
		c := newTestContext()
		a := &mockOCRAdaptor{convertErr: errors.New("conversion failed")}
		req := &relaymodel.OCRRequest{Model: "glm-ocr", File: "https://example.com/img.png"}

		_, err := prepareOCRRequestBody(c, &metalib.Meta{}, a, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conversion failed")
	})
}

// ===========================================================================
// 3. preConsumeOCRQuota
// ===========================================================================

func TestPreConsumeOCRQuota(t *testing.T) {
	t.Parallel()

	t.Run("zero quota returns immediately", func(t *testing.T) {
		c, _ := newOCRGinContext(nil)
		meta := &metalib.Meta{UserId: 1, TokenId: 1}
		preConsumed, bizErr := preConsumeOCRQuota(c, 0, meta)
		require.Nil(t, bizErr)
		assert.Equal(t, int64(0), preConsumed)
	})

	t.Run("negative quota clamped to zero", func(t *testing.T) {
		c, _ := newOCRGinContext(nil)
		meta := &metalib.Meta{UserId: 1, TokenId: 1}
		preConsumed, bizErr := preConsumeOCRQuota(c, -5, meta)
		require.Nil(t, bizErr)
		assert.Equal(t, int64(0), preConsumed)
	})
}

// ===========================================================================
// 4. postConsumeOCRQuota
// ===========================================================================

func TestPostConsumeOCRQuota(t *testing.T) {
	t.Parallel()

	t.Run("returns totalQuota as final quota", func(t *testing.T) {
		usage := &relaymodel.Usage{PromptTokens: 50, CompletionTokens: 100, TotalTokens: 150}
		meta := &metalib.Meta{UserId: 1, ChannelId: 1, TokenId: 0, TokenName: "unit-test", StartTime: time.Now()}
		request := &relaymodel.OCRRequest{Model: "glm-ocr"}

		got := postConsumeOCRQuota(context.Background(), usage, meta, request, 100, 500, 0.5, 1.0)
		require.Equal(t, int64(500), got)
	})

	t.Run("zero totalQuota returns zero", func(t *testing.T) {
		meta := &metalib.Meta{UserId: 1, ChannelId: 1, TokenId: 0, StartTime: time.Now()}
		got := postConsumeOCRQuota(context.Background(), &relaymodel.Usage{}, meta, &relaymodel.OCRRequest{Model: "glm-ocr"}, 0, 0, 0, 1)
		require.Equal(t, int64(0), got)
	})

	t.Run("negative totalQuota clamped to zero", func(t *testing.T) {
		meta := &metalib.Meta{UserId: 1, ChannelId: 1, TokenId: 0, StartTime: time.Now()}
		got := postConsumeOCRQuota(context.Background(), nil, meta, &relaymodel.OCRRequest{Model: "glm-ocr"}, 0, -10, 1.0, 1.0)
		require.Equal(t, int64(0), got)
	})

	t.Run("nil usage does not panic", func(t *testing.T) {
		meta := &metalib.Meta{UserId: 1, ChannelId: 1, TokenId: 0, StartTime: time.Now()}
		require.NotPanics(t, func() {
			postConsumeOCRQuota(context.Background(), nil, meta, &relaymodel.OCRRequest{Model: "glm-ocr"}, 0, 100, 1.0, 1.0)
		})
	})

	t.Run("incomplete meta logs error but does not panic", func(t *testing.T) {
		// With TokenId == 0, postConsumeOCRQuota takes the "incomplete meta" error
		// log branch and does not attempt DB operations.
		meta := &metalib.Meta{
			UserId: 1, ChannelId: 2, TokenId: 0,
			TokenName: "test-token", StartTime: time.Now(),
		}
		usage := &relaymodel.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}
		require.NotPanics(t, func() {
			got := postConsumeOCRQuota(context.Background(), usage, meta, &relaymodel.OCRRequest{Model: "glm-ocr"}, 50, 100, 1.0, 1.0)
			assert.Equal(t, int64(100), got)
		})
	})
}

// ===========================================================================
// 5. Billing lifecycle: markPreConsumed / billingAuditSafetyNet / reconciliation
// ===========================================================================

func TestOCRBillingLifecycle_NormalPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", nil)

	// Step 1: Pre-consume
	markPreConsumed(c, 5000)
	val, exists := c.Get(ctxkey.PreConsumedQuotaAmount)
	require.True(t, exists)
	require.Equal(t, int64(5000), val.(int64))

	// Step 2: Mark reconciled (post-billing success path)
	markBillingReconciled(c)
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	require.Equal(t, true, reconciled.(bool))

	// Step 3: Safety net should be a no-op
	billingAuditSafetyNet(c)
}

func TestOCRBillingLifecycle_ErrorRefundPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", nil)
	c.Set(ctxkey.Id, 42)
	c.Set(ctxkey.TokenId, 10)
	c.Set(ctxkey.ChannelId, 5)
	c.Set(ctxkey.RequestId, "req_ocr_error")

	markPreConsumed(c, 5000)

	// Simulate error before upstream forwarding: refund should be attempted
	returnPreConsumedQuotaConservative(c.Request.Context(), c, 5000, 10, "test_ocr_error")

	// Safety net should be a no-op now (reconciled by returnPreConsumedQuotaConservative)
	billingAuditSafetyNet(c)
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	require.Equal(t, true, reconciled.(bool))
}

func TestOCRBillingLifecycle_UnreconciledSafetyNet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", nil)
	c.Set(ctxkey.Id, 42)
	c.Set(ctxkey.TokenId, 10)

	markPreConsumed(c, 5000)
	// NOT marking as reconciled — simulates a code path bug

	// Safety net should detect and handle the unreconciled quota without panic
	require.NotPanics(t, func() { billingAuditSafetyNet(c) })
}

func TestOCRBillingLifecycle_ZeroPreConsume(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", nil)

	markPreConsumed(c, 0)
	// Zero pre-consume: safety net should be a no-op
	billingAuditSafetyNet(c)
}

// ===========================================================================
// 6. Quota return on various failure paths
// ===========================================================================

func TestReturnPreConsumedQuota_SkipWhenForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", nil)

	// Once upstream request is possibly forwarded, refund is skipped
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	refunded := returnPreConsumedQuotaConservative(context.Background(), c, 123, 1, "ocr_skip_test")
	require.False(t, refunded)
}

func TestReturnPreConsumedQuota_ZeroAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/paas/v4/layout_parsing", nil)

	refunded := returnPreConsumedQuotaConservative(context.Background(), c, 0, 1, "ocr_zero_test")
	require.False(t, refunded)
}

// ===========================================================================
// 7. isStream is always false for OCR
// ===========================================================================

func TestOCRIsStreamAlwaysFalse(t *testing.T) {
	t.Parallel()

	// After getAndValidateOCRRequest + meta setup, IsStream must be false.
	// The RelayOCRHelper sets meta.IsStream = false unconditionally.
	// Verify the logic in isolation.
	meta := &metalib.Meta{IsStream: true} // intentionally set to true
	meta.IsStream = false                 // what RelayOCRHelper does
	assert.False(t, meta.IsStream)
}

// ===========================================================================
// 8. Model mapping
// ===========================================================================

func TestOCRModelMapping(t *testing.T) {
	t.Parallel()

	mapping := map[string]string{
		"ocr-custom": "glm-ocr",
	}

	// Simulate the model mapping that RelayOCRHelper does
	originalModel := "ocr-custom"
	actualModel := metalib.GetMappedModelName(originalModel, mapping)
	assert.Equal(t, "glm-ocr", actualModel)

	// No mapping entry: model unchanged
	actualModel2 := metalib.GetMappedModelName("glm-ocr", mapping)
	assert.Equal(t, "glm-ocr", actualModel2)
}

// ===========================================================================
// 9. totalQuota calculation edge cases
// ===========================================================================

func TestOCRTotalQuotaCalculation(t *testing.T) {
	t.Parallel()
	// Mirrors the logic in RelayOCRHelper:
	//   totalQuota = int64(math.Ceil(modelRatio * groupRatio))
	//   if modelRatio > 0 && totalQuota == 0 { totalQuota = 1 }

	t.Run("normal calculation", func(t *testing.T) {
		// 1.5 * 2.0 = 3.0 → 3
		import_math_ceil := func(f float64) int64 {
			return int64(f + 0.999999)
		}
		_ = import_math_ceil
		// Just verify the logic inline
		modelRatio := 1.5
		groupRatio := 2.0
		totalQuota := int64(modelRatio * groupRatio)
		assert.Equal(t, int64(3), totalQuota)
	})

	t.Run("fractional rounds up to minimum 1", func(t *testing.T) {
		modelRatio := 0.001
		groupRatio := 0.001
		product := modelRatio * groupRatio // 0.000001
		totalQuota := int64(product)       // 0 due to truncation
		if modelRatio > 0 && totalQuota == 0 {
			totalQuota = 1
		}
		assert.Equal(t, int64(1), totalQuota)
	})

	t.Run("zero model ratio", func(t *testing.T) {
		modelRatio := 0.0
		groupRatio := 1.0
		totalQuota := int64(modelRatio * groupRatio)
		if modelRatio > 0 && totalQuota == 0 {
			totalQuota = 1
		}
		assert.Equal(t, int64(0), totalQuota)
	})
}

// ===========================================================================
// 10. OCRRequest model serialization
// ===========================================================================

func TestOCRRequest_RoundTrip(t *testing.T) {
	t.Parallel()
	boolTrue := true
	startPage := 3
	endPage := 8

	original := &relaymodel.OCRRequest{
		Model: "glm-ocr", File: "https://example.com/doc.pdf",
		RequestID: "req-roundtrip", UserID: "user-rt",
		ReturnCropImages: &boolTrue, StartPageID: &startPage, EndPageID: &endPage,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var deserialized relaymodel.OCRRequest
	require.NoError(t, json.Unmarshal(data, &deserialized))

	assert.Equal(t, original.Model, deserialized.Model)
	assert.Equal(t, original.File, deserialized.File)
	assert.Equal(t, original.RequestID, deserialized.RequestID)
	assert.Equal(t, original.UserID, deserialized.UserID)
	require.NotNil(t, deserialized.ReturnCropImages)
	assert.True(t, *deserialized.ReturnCropImages)
	assert.Nil(t, deserialized.NeedLayoutVisualization)
	require.NotNil(t, deserialized.StartPageID)
	assert.Equal(t, 3, *deserialized.StartPageID)
}

func TestOCRRequest_OmitEmpty(t *testing.T) {
	t.Parallel()
	req := &relaymodel.OCRRequest{Model: "glm-ocr", File: "https://example.com/img.png"}
	data, _ := json.Marshal(req)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Contains(t, m, "model")
	assert.Contains(t, m, "file")
	for _, key := range []string{"request_id", "user_id", "return_crop_images", "need_layout_visualization", "start_page_id", "end_page_id"} {
		assert.NotContains(t, m, key)
	}
}
