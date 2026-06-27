package cerebras

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
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
	base := "https://api.cerebras.ai/v1"

	cases := map[string]string{
		"/v1/chat/completions": base + "/chat/completions",
		// Claude Messages are converted to OpenAI chat completions, so they must
		// resolve to the chat completions endpoint.
		"/v1/messages": base + "/chat/completions",
	}

	for path, want := range cases {
		got, err := a.GetRequestURL(&meta.Meta{
			BaseURL:        base,
			RequestURLPath: path,
			ChannelType:    channeltype.Cerebras,
		})
		require.NoError(t, err, "path=%s", path)
		require.Equal(t, want, got, "path=%s", path)
	}
}

// TestChannelTypeMapsToCerebrasAPIType guards the channeltype -> apitype wiring
// so the dedicated cerebras adaptor (and its pricing) is actually selected at
// runtime instead of falling through to the generic OpenAI adaptor.
func TestChannelTypeMapsToCerebrasAPIType(t *testing.T) {
	t.Parallel()
	require.Equal(t, apitype.Cerebras, channeltype.ToAPIType(channeltype.Cerebras))
	require.Equal(t, "cerebras", channeltype.IdToName(channeltype.Cerebras))
}

// TestPricingMatchesPublishedRates locks in the per-token rates derived from
// Cerebras' official model cards so an accidental edit is caught.
func TestPricingMatchesPublishedRates(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}

	// gpt-oss-120b: $0.35 in / $0.75 out per 1M tokens.
	require.Equal(t, 0.35*ratio.MilliTokensUsd, a.GetModelRatio("gpt-oss-120b"))
	require.InDelta(t, 0.75/0.35, a.GetCompletionRatio("gpt-oss-120b"), 1e-9)

	// zai-glm-4.7: $2.25 in / $2.75 out per 1M tokens.
	require.Equal(t, 2.25*ratio.MilliTokensUsd, a.GetModelRatio("zai-glm-4.7"))
	require.InDelta(t, 2.75/2.25, a.GetCompletionRatio("zai-glm-4.7"), 1e-9)

	// Unknown model falls back to DefaultPricingMethods (non-zero default).
	unknown := "cerebras/does-not-exist"
	require.Greater(t, a.GetModelRatio(unknown), 0.0)
	require.Equal(t, 1.0, a.GetCompletionRatio(unknown))
}

// TestKnownModelsPresent locks in the registered model IDs so an accidental
// rename/removal is caught.
func TestKnownModelsPresent(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"gpt-oss-120b",
		"zai-glm-4.7",
	} {
		cfg, ok := ModelRatios[id]
		require.True(t, ok, "expected catalog model %q to be registered", id)
		require.Greater(t, cfg.Ratio, 0.0, "model %q must carry a published per-token price", id)
	}
}

func TestGetModelList_Nonempty(t *testing.T) {
	t.Parallel()

	models := (&Adaptor{}).GetModelList()
	require.NotEmpty(t, models)
	require.Equal(t, len(ModelRatios), len(models))
}

// TestConvertRequest_PreservesReasoningEffort verifies that, unlike providers
// whose chat endpoint rejects reasoning_effort, Cerebras accepts it as a
// standard parameter, so the adaptor must forward it unchanged.
func TestConvertRequest_PreservesReasoningEffort(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	effort := "high"
	req := &model.GeneralOpenAIRequest{
		Model:           "gpt-oss-120b",
		ReasoningEffort: &effort,
	}

	converted, err := (&Adaptor{}).ConvertRequest(c, 0, req)
	require.NoError(t, err)
	out, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, out.ReasoningEffort, "reasoning_effort must be preserved for Cerebras")
	require.Equal(t, "high", *out.ReasoningEffort)
}
