package ali

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func float64PtrAli(v float64) *float64 {
	return &v
}

func TestConvertRequestClampsTopP(t *testing.T) {
	t.Parallel()
	req := model.GeneralOpenAIRequest{
		Model: "qwen-plus-internet",
		TopP:  float64PtrAli(1.5),
	}

	converted := ConvertRequest(req)
	require.NotNil(t, converted.Parameters.TopP, "expected TopP to be populated")

	diff := math.Abs(*converted.Parameters.TopP - 0.9999)
	require.LessOrEqual(t, diff, 1e-9, "expected TopP to be clamped to 0.9999, got %v", *converted.Parameters.TopP)
}

func TestConvertRequestLeavesNilTopPUnchanged(t *testing.T) {
	t.Parallel()
	req := model.GeneralOpenAIRequest{
		Model: "qwen-plus",
	}

	converted := ConvertRequest(req)
	require.Nil(t, converted.Parameters.TopP, "expected TopP to remain nil when not provided")
}

func TestIsQwenImageSyncModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  bool
	}{
		{"qwen-image-2.0-pro", true},
		{"qwen-image-2.0", true},
		{"qwen-image-max", true},
		{"qwen-image-plus", false},
		{"qwen-image", false},
		{"wanx-v1", false},
		{"ali-stable-diffusion-xl", false},
		{"qwen-max", false},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, isQwenImageSyncModel(tc.model), "model: %s", tc.model)
	}
}

func TestConvertQwenImageSyncRequest(t *testing.T) {
	t.Parallel()

	req := model.ImageRequest{
		Model:  "qwen-image-2.0-pro",
		Prompt: "a cat on the moon",
		Size:   "1024x1024",
		N:      2,
	}

	result := ConvertQwenImageSyncRequest(req)

	require.Equal(t, "qwen-image-2.0-pro", result.Model)
	require.Len(t, result.Input.Messages, 1)
	require.Equal(t, "user", result.Input.Messages[0].Role)
	require.Len(t, result.Input.Messages[0].Content, 1)
	require.Equal(t, "a cat on the moon", result.Input.Messages[0].Content[0].Text)
	require.Equal(t, "1024*1024", result.Parameters.Size)
	require.Equal(t, 2, result.Parameters.N)
	require.NotNil(t, result.Parameters.Watermark)
	require.False(t, *result.Parameters.Watermark)
}

func TestConvertQwenImageSyncRequestDefaults(t *testing.T) {
	t.Parallel()

	req := model.ImageRequest{
		Model:  "qwen-image-2.0",
		Prompt: "hello",
	}

	result := ConvertQwenImageSyncRequest(req)

	require.Equal(t, "1024*1024", result.Parameters.Size)
	require.Equal(t, 1, result.Parameters.N)
}

func TestConvertImageRequestLegacy(t *testing.T) {
	t.Parallel()

	format := "url"
	req := model.ImageRequest{
		Model:          "wanx-v1",
		Prompt:         "sunset",
		Size:           "1024x1024",
		N:              1,
		ResponseFormat: &format,
	}

	result := ConvertImageRequest(req)

	require.Equal(t, "wanx-v1", result.Model)
	require.Equal(t, "sunset", result.Input.Prompt)
	require.Equal(t, "1024*1024", result.Parameters.Size)
	require.Equal(t, 1, result.Parameters.N)
	require.Equal(t, "url", result.ResponseFormat)
}
