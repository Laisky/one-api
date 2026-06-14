package nvidia

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestGetRequestURL_CollapsesV1Prefix verifies that the documented base URL,
// which carries a /v1 version suffix, does not produce a doubled /v1 in the
// final request URL, and that Claude Messages requests are routed to chat
// completions.
func TestGetRequestURL_CollapsesV1Prefix(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	base := "https://integrate.api.nvidia.com/v1"

	cases := map[string]string{
		"/v1/chat/completions": base + "/chat/completions",
		"/v1/embeddings":       base + "/embeddings",
		// Claude Messages are converted to OpenAI chat completions, so they must
		// resolve to the chat completions endpoint.
		"/v1/messages": base + "/chat/completions",
	}

	for path, want := range cases {
		got, err := a.GetRequestURL(&meta.Meta{
			BaseURL:        base,
			RequestURLPath: path,
			ChannelType:    channeltype.NVIDIA,
		})
		require.NoError(t, err, "path=%s", path)
		require.Equal(t, want, got, "path=%s", path)
	}
}

// TestChannelTypeMapsToNvidiaAPIType guards the channeltype -> apitype wiring so
// the dedicated nvidia adaptor (and its free pricing) is actually selected at
// runtime instead of falling through to the generic OpenAI adaptor.
func TestChannelTypeMapsToNvidiaAPIType(t *testing.T) {
	t.Parallel()
	require.Equal(t, apitype.NVIDIA, channeltype.ToAPIType(channeltype.NVIDIA))
	require.Equal(t, "nvidia", channeltype.IdToName(channeltype.NVIDIA))
}

// TestPricingIsFree verifies the free-billing contract: every curated model
// resolves to Ratio 0 (genuinely free, via the existence check in the billing
// resolver) while unknown models fall back to the non-zero default.
func TestPricingIsFree(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}

	for modelName, cfg := range ModelRatios {
		require.Equal(t, 0.0, cfg.Ratio, "model %q must be registered free", modelName)
		require.Equal(t, 1.0, cfg.CompletionRatio, "model %q must keep CompletionRatio 1", modelName)
		require.Equal(t, 0.0, a.GetModelRatio(modelName), "GetModelRatio must report free for %q", modelName)
		require.Equal(t, 1.0, a.GetCompletionRatio(modelName), "GetCompletionRatio must report 1 for %q", modelName)
	}

	// Unknown model falls back to DefaultPricingMethods (non-zero default).
	unknown := "nvidia/does-not-exist"
	require.Greater(t, a.GetModelRatio(unknown), 0.0)
	require.Equal(t, 1.0, a.GetCompletionRatio(unknown))
}

// TestKnownFlagshipModelsPresent locks in a handful of headline IDs so an
// accidental rename/removal is caught.
func TestKnownFlagshipModelsPresent(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"nvidia/nemotron-3-ultra-550b-a55b",
		"nvidia/llama-3.3-nemotron-super-49b-v1.5",
		"meta/llama-3.3-70b-instruct",
		"deepseek-ai/deepseek-v4-flash",
		"openai/gpt-oss-120b",
	} {
		_, ok := ModelRatios[id]
		require.True(t, ok, "expected catalog model %q to be registered", id)
	}
}

func TestGetModelList_Nonempty(t *testing.T) {
	t.Parallel()

	models := (&Adaptor{}).GetModelList()
	require.NotEmpty(t, models)
	require.Equal(t, len(ModelRatios), len(models))
}

// TestConvertRequest_DropsReasoningEffort verifies the single normalization the
// adaptor performs: NVIDIA's chat completions endpoint rejects OpenAI's
// reasoning_effort parameter, so it must be stripped before forwarding.
func TestConvertRequest_DropsReasoningEffort(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	effort := "high"
	req := &model.GeneralOpenAIRequest{
		Model:           "nvidia/nemotron-3-ultra-550b-a55b",
		ReasoningEffort: &effort,
	}

	converted, err := (&Adaptor{}).ConvertRequest(c, 0, req)
	require.NoError(t, err)
	out, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Nil(t, out.ReasoningEffort, "reasoning_effort must be dropped for NVIDIA")
}
