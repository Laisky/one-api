package zhipu

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func float64PtrZhipu(v float64) *float64 {
	return &v
}

func newZhipuContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Request = req
	return c
}

func TestConvertRequestClampsParametersV4(t *testing.T) {
	adaptor := &Adaptor{}
	req := &model.GeneralOpenAIRequest{
		Model:       "glm-4",
		TopP:        float64PtrZhipu(2.0),
		Temperature: float64PtrZhipu(-0.5),
	}

	c := newZhipuContext()

	convertedAny, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err, "ConvertRequest returned error")

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok, "expected v4 conversion to return GeneralOpenAIRequest, got %T", convertedAny)

	require.NotNil(t, converted.TopP, "expected TopP to be non-nil")
	require.Equal(t, float64(1), *converted.TopP, "expected TopP to be clamped to 1")

	require.NotNil(t, converted.Temperature, "expected Temperature to be non-nil")
	require.Equal(t, float64(0), *converted.Temperature, "expected Temperature to be clamped to 0")
}

func TestConvertRequestClampsParametersV3(t *testing.T) {
	adaptor := &Adaptor{}
	req := &model.GeneralOpenAIRequest{
		Model:       "chatglm-3",
		TopP:        float64PtrZhipu(-0.3),
		Temperature: float64PtrZhipu(1.5),
	}

	c := newZhipuContext()

	convertedAny, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err, "ConvertRequest returned error")

	converted, ok := convertedAny.(*Request)
	require.True(t, ok, "expected v3 conversion to return *Request, got %T", convertedAny)

	require.NotNil(t, converted.TopP, "expected TopP to be non-nil")
	require.Equal(t, float64(0), *converted.TopP, "expected TopP to be clamped to 0")

	require.NotNil(t, converted.Temperature, "expected Temperature to be non-nil")
	require.Equal(t, float64(1), *converted.Temperature, "expected Temperature to be clamped to 1")
}
