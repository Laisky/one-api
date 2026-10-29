package controller

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestTypedMixedBridgeFundsTextBeforeDelivery checks shared non-Gemini quota enforcement before emitting a mixed delta.
func TestTypedMixedBridgeFundsTextBeforeDelivery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		balance int64
		allowed bool
	}{{"insufficient", 2, false}, {"affordable", 100, true}} {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, tc.balance, false)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			const modelName = "synthetic-non-gemini-text"
			tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{UserID: fallbackUserID, TokenID: fallbackTokenID, ModelName: modelName, ModelRatio: 1, GroupRatio: 1, Ctx: context.Background(), ChannelModelConfigs: map[string]dbmodel.ModelConfigLocal{modelName: {Ratio: 1, CompletionRatio: 1}}, TokenCounter: func(s, _ string) int { return len(s) }})
			streaming.StoreTracker(c, tracker)
			c.Set(ctxkey.ResponseStreamRewriteHandler, newChatToResponseStreamBridge(c, &metalib.Meta{ActualModelName: modelName, ChannelType: channeltype.OpenAI}, &openai.ResponseAPIRequest{Model: modelName}))
			text := "hello"
			url := "data:image/png;base64," + strings.Repeat("A", 1000)
			chunk := &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Delta: model.Message{Content: []model.MessageContent{{Type: model.ContentTypeText, Text: &text}, {Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: url}}}}}}}
			err := openai_compatible.RenderStreamChunkWithBridge(c, chunk)
			if !tc.allowed {
				require.Error(t, err)
				require.NotContains(t, rec.Body.String(), "hello")
				return
			}
			require.NoError(t, err)
			require.Contains(t, rec.Body.String(), "hello")
			require.Contains(t, rec.Body.String(), url)
			require.Equal(t, 5, tracker.UsageSnapshot().CompletionTokens)
		})
	}
}
