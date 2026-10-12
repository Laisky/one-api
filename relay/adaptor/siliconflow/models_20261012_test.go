package siliconflow_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/siliconflow"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
)

// TestSiliconFlowGLM53Flash checks public discovery, exact native-currency
// prices, conservative metadata and request preservation using t.
func TestSiliconFlowGLM53Flash(t *testing.T) {
	t.Parallel()
	const id = "zai-org/GLM-5.3-Flash"
	a := &siliconflow.Adaptor{}
	cfg, exists := a.GetDefaultModelPricing()[id]
	require.True(t, exists)
	require.Contains(t, a.GetModelList(), id)
	require.Contains(t, siliconflow.ModelList, id)
	require.InDelta(t, 0.80, a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 0.23, cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 2.80, a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.Empty(t, cfg.SupportedReasoningEfforts)
	require.Empty(t, cfg.TimeWindows, "do not import another provider's promotion")
	require.Zero(t, cfg.ContextLength, "unknown hosted limit is not a creator-model guess")
	require.Zero(t, cfg.MaxOutputTokens)
	require.Zero(t, cfg.MaxTokens)
	require.Nil(t, cfg.PerCall, "chat is token-priced")
	var req model.GeneralOpenAIRequest
	const payload = `{"model":"zai-org/GLM-5.3-Flash","messages":[{"role":"user","content":"Hello"}],"max_tokens":64,"stream":true}`
	require.NoError(t, json.Unmarshal([]byte(payload), &req))
	before, err := json.Marshal(req)
	require.NoError(t, err)
	converted, err := a.ConvertRequest(nil, 0, &req)
	require.NoError(t, err)
	after, err := json.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	full := a.GetDefaultModelPricing()["zai-org/GLM-5.3"]
	require.InDelta(t, 8.0, full.Ratio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 2.0, full.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 28.0, full.Ratio*full.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
}
