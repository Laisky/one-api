package ali

import (
	"math"
	"testing"

	"github.com/songquanpeng/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

func float64PtrAli(v float64) *float64 {
	return &v
}

func TestConvertRequestClampsTopP(t *testing.T) {
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
	req := model.GeneralOpenAIRequest{
		Model: "qwen-plus",
	}

	converted := ConvertRequest(req)
	require.Nil(t, converted.Parameters.TopP, "expected TopP to remain nil when not provided")
}
