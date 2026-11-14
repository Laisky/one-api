package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/vertexai/deepseek"
	"github.com/Laisky/one-api/relay/meta"
)

// TestCurrentMaaSRoutes pins publisher-specific endpoint selection without inference calls.
func TestCurrentMaaSRoutes(t *testing.T) {
	for _, tc := range []struct{ name, host, region string }{
		{"deepseek-ai/deepseek-v4.1-flash-maas", "aiplatform.googleapis.com", "global"},
		{"zai-org/glm-5.2-maas", "aiplatform.googleapis.com", "global"},
		{"google/gemma-4-26b-a4b-it-maas", "aiplatform.googleapis.com", "global"},
		{"meta/llama-4-maverick-17b-128e-instruct-maas", "us-east5-aiplatform.googleapis.com", "us-east5"},
		{"meta/llama-4-scout-17b-16e-instruct-maas", "us-east5-aiplatform.googleapis.com", "us-east5"},
		// Existing aliases keep their explicit route, independent of current catalog advertising.
		{"deepseek-ai/deepseek-r1-0528-maas", "us-central1-aiplatform.googleapis.com", "us-central1"},
		{"openai/gpt-oss-120b-maas", "aiplatform.googleapis.com", "global"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &meta.Meta{ActualModelName: tc.name}
			m.Config.VertexAIProjectID = "offline-project"
			got, err := (&Adaptor{}).GetRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, "https://"+tc.host+"/v1/projects/offline-project/locations/"+tc.region+"/endpoints/openapi/chat/completions", got)
			require.NotNil(t, GetAdaptor(tc.name))
		})
	}
	require.Nil(t, GetAdaptor("openai/gpt-6.1-sol"), "Responses-only preview is not a Chat Completions model")
	require.Nil(t, GetAdaptor("zai-org/unverified-maas"))
	cfg, ok := deepseek.ModelRatios["deepseek-ai/deepseek-v4.1-flash-maas"]
	require.True(t, ok)
	require.Equal(t, int32(1048576), cfg.ContextLength)
	require.Equal(t, int32(1048576), cfg.MaxOutputTokens)
}
