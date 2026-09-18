package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiLiveEphemeralRejectionExplainsSupportedTransport exercises the HTTP
// error clients actually receive. Parameters: t is the test handle. Returns:
// none. Gemini-compatible channels must not suggest minting OpenAI tokens.
func TestGeminiLiveEphemeralRejectionExplainsSupportedTransport(t *testing.T) {
	t.Parallel()
	for _, channel := range []int{channeltype.Gemini, channeltype.GeminiOpenAICompatible} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", strings.NewReader(`{"model":"gemini-3.8-live"}`))
		gmw.SetLogger(c, logger.Logger)
		m := &meta.Meta{ChannelType: channel, APIType: apitype.OpenAI, Mode: relaymode.Realtime, ActualModelName: "gemini-3.8-live", StartTime: time.Now()}
		meta.Set2Context(c, m)
		RelayRealtimeSessions(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
		var payload struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
		require.Equal(t, "realtime_sessions_unsupported", payload.Error.Code)
		require.Contains(t, payload.Error.Message, "Gemini Live")
		require.Contains(t, payload.Error.Message, "WebSocket")
		require.Contains(t, payload.Error.Message, "/v1/realtime")
		require.Contains(t, payload.Error.Message, "one-api")
	}
}

// TestGeminiRejectedFinalReceiptRetainsSettlementFloor verifies production
// settlement after a partial receipt, malformed final and later valid turn.
// Parameters: t is the test handle. Returns: none; no pricing function is mocked.
func TestGeminiRejectedFinalReceiptRetainsSettlementFloor(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gemini-3.8-live", "gemini-3.8-live-extended-thinking"} {
		t.Run(name, func(t *testing.T) {
			g := realtime.NewGeminiLedger()
			const partial = `{"serverContent":{"modelTurn":{}},"usageMetadata":{"promptTokenCount":100,"responseTokenCount":200,"totalTokenCount":300,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":100}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100}]}}`
			require.NoError(t, g.Observe([]byte(partial)))
			rejectedErr := g.Observe([]byte(`{"serverContent":{"turnComplete":true},"usageMetadata":{"totalTokenCount":-1}}`))
			require.NoError(t, g.Observe([]byte(strings.Replace(partial, `"modelTurn":{}`, `"modelTurn":{},"turnComplete":true`, 1))))
			ledger := g.Finish(false)
			usage := &relaymodel.Usage{Realtime: ledger}
			result, metadata := prepareRealtimeReceiptSettlement(quota.ComputeInput{Usage: usage, ModelName: name, ModelRatio: .75 * ratio.MilliTokensUsd, GroupRatio: 1, PricingAdaptor: &gemini.Adaptor{}}, 22500, logger.Logger)
			t.Logf("measured_records=%d settlement=%d complete=%v rejected_error=%v", len(ledger.Records), result.TotalQuota, metadata["realtime_billing_complete"], rejectedErr)
			require.Len(t, ledger.Records, 2)
			require.EqualValues(t, 22500, result.TotalQuota, "do not settle incomplete usage as an exact smaller bill")
			require.Equal(t, false, metadata["realtime_billing_complete"])
			require.Equal(t, true, metadata[model.LogMetadataKeyEstimatedCharge])
			require.Error(t, rejectedErr)
		})
	}
}
