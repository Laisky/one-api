package gemini

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestGeminiImageUsageEstimateKeepsReservation checks missing-receipt provenance while authoritative stream receipts replace the image estimate.
func TestGeminiImageUsageEstimateKeepsReservation(t *testing.T) {
	for _, measured := range []bool{false, true} {
		name := "missing"
		if measured {
			name = "measured"
		}
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			payload := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}]}\n\ndata: [DONE]\n\n"
			if measured {
				payload = geminiStreamSSE(t, []string{"ok"})
			}
			usage, apiErr := (&Adaptor{}).DoResponse(c, newGeminiStreamResp(payload), &meta.Meta{Mode: relaymode.ChatCompletions, IsStream: true, ActualModelName: "gemini-2.5-flash", PromptTokens: 765})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			if measured {
				require.Equal(t, 3, usage.PromptTokens)
				require.Empty(t, usage.BillingEstimateReason)
			} else {
				require.Equal(t, 765, usage.PromptTokens)
				require.NotEmpty(t, usage.BillingEstimateReason)
			}
		})
	}
}
