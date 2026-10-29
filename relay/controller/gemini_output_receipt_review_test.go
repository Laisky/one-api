package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// TestGeminiUnrepresentableImageTokensRemainBillable exercises the actual Gemini
// response handlers and controller settlement: an aggregate IMAGE receipt must
// not be fully excluded if one upstream media part could not be represented.
func TestGeminiUnrepresentableImageTokensRemainBillable(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name           string
			secondMime     string
			wantImages     int
			wantIncomplete bool
			wantExcluded   int
		}{
			{"all_images_represented", "image/png", 2, false, 2240},
			{"one_unrepresentable_media", "application/octet-stream", 1, true, 0},
		} {
			t.Run(fmt.Sprintf("%t/%s", stream, tc.name), func(t *testing.T) {
				payload := fmt.Sprintf("{\"candidates\":[{\"content\":{\"parts\":[{\"inlineData\":{\"mimeType\":\"image/png\",\"data\":\"YQ==\"}},{\"inlineData\":{\"mimeType\":\"%s\",\"data\":\"Yg==\"}}]}}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2240,\"totalTokenCount\":2241,\"candidatesTokensDetails\":[{\"modality\":\"IMAGE\",\"tokenCount\":2240}]}}", tc.secondMime)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				c.Set(ctxkey.ChannelRatio, 1.0)
				var usage *model.Usage
				if stream {
					resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n"))}
					apiErr, _, got := gemini.StreamHandler(c, resp)
					require.Nil(t, apiErr)
					usage = got
				} else {
					resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
						Body: io.NopCloser(strings.NewReader(payload))}
					apiErr, got := gemini.Handler(c, resp, 1, "gemini-nano-banana-2.1")
					require.Nil(t, apiErr)
					usage = got
				}
				require.NotNil(t, usage)
				require.Equal(t, 2240, usage.CandidateImageTokens)
				require.Equal(t, 2240, usage.CompletionTokens)
				require.Equal(t, tc.wantImages, c.GetInt(ctxkey.OutputImageCount))
				require.Equal(t, tc.wantIncomplete, c.GetBool(ctxkey.GeminiOutputIncomplete))

				m := &meta.Meta{ActualModelName: "gemini-nano-banana-2.1",
					ChannelType: channeltype.VertextAI, APIType: apitype.VertexAI}
				applyOutputImageCharges(c, &usage, m)
				require.Positive(t, usage.ToolsCost)
				require.Equal(t, tc.wantExcluded, usage.ImageTokensBilledSeparately,
					"unrecognized media must retain conservative ordinary token billing")
				require.Equal(t, 2240, usage.CompletionTokens, "never rewrite provider usage")
				// Assert the final shared quota calculation, not only the intermediate billing marker.
				input := quota.ComputeInput{Usage: usage, ModelName: "gemini-nano-banana-2.1", ModelRatio: 100, GroupRatio: 1}
				charged := quota.Compute(input)
				input.Usage = &model.Usage{PromptTokens: 1, CompletionTokens: 2240 - tc.wantExcluded, ToolsCost: usage.ToolsCost}
				expected := quota.Compute(input)
				require.Equal(t, expected.TotalQuota, charged.TotalQuota)
			})
		}
	}
}
