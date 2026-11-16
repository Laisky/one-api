package siliconflow_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/siliconflow"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestMOSSImageNativeContract verifies the native CNY image tariff, exact wire
// ID and single-output request/receipt contract used by the image controller.
// It takes t and returns no values without performing image generation.
func TestMOSSImageNativeContract(t *testing.T) {
	t.Parallel()
	const id = "OpenMOSS-Team/MOSS-Image"
	a := &siliconflow.Adaptor{}
	require.Contains(t, a.GetModelList(), id)
	require.Contains(t, siliconflow.ModelList, id)
	cfg, _ := pricing.ResolveImagePricing(id, nil, a, time.Now().UTC())
	require.NotNil(t, cfg)
	require.InDelta(t, .30/ratio.ExchangeRateRmb, cfg.PricePerImageUsd, 1e-12)
	require.Equal(t, 1, cfg.MinImages)
	require.Equal(t, 1, cfg.MaxImages)
	require.Equal(t, []string{"image"}, a.GetDefaultModelPricing()[id].OutputModalities)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"public-alias","prompt":"A poster","image_size":"1024x1024","n":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	request := &model.ImageRequest{Model: id, Prompt: "A poster"}
	require.NoError(t, openai.PrepareSiliconFlowImage(c, request))
	converted, err := openai.ConvertSiliconFlowImage(c, request)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Equal(t, id, wire["model"])
	require.Equal(t, "1024x1024", wire["image_size"])
	require.Equal(t, 1, request.N)
	require.NotContains(t, wire, "n")
	apiErr, _ := openai.SiliconFlowImageHandler(c, &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{"images":[{"url":"https://example.com/generated.png"}]}`)),
	})
	require.Nil(t, apiErr)
	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Len(t, result.Data, 1)
	require.Equal(t, "https://example.com/generated.png", result.Data[0].URL)

	invalid, _ := gin.CreateTestContext(httptest.NewRecorder())
	invalid.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"OpenMOSS-Team/MOSS-Image","prompt":"A poster","n":2}`))
	invalid.Request.Header.Set("Content-Type", "application/json")
	require.Error(t, openai.PrepareSiliconFlowImage(invalid, &model.ImageRequest{Model: id, Prompt: "A poster"}))
}

// TestNovemberRetirementsRetainCatalogEntries verifies announced retirements
// remain discoverable without inventing replacement IDs. It takes t and returns nothing.
func TestNovemberRetirementsRetainCatalogEntries(t *testing.T) {
	t.Parallel()
	a := &siliconflow.Adaptor{}
	for id, date := range map[string]string{
		"Pro/zai-org/GLM-5.1":           "2026-11-10",
		"tencent/Hunyuan-A13B-Instruct": "2026-11-16",
		"inclusionAI/Ling-mini-2.0":     "2026-11-16",
		"stepfun-ai/Step-3.5-Flash":     "2026-11-16",
		"zai-org/GLM-4.5V":              "2026-11-16",
		"Qwen/Qwen3.5-122B-A10B":        "2026-11-16",
	} {
		require.Contains(t, a.GetModelList(), id)
		require.Contains(t, a.GetDefaultModelPricing()[id].Description, date)
		require.Greater(t, a.GetModelRatio(id), 0.0)
	}
	require.NotContains(t, a.GetModelList(), "deepseek-ai/DeepSeek-V4.1-Flash-1017", "SiliconFlow keeps the unversioned ID upstream")
}
