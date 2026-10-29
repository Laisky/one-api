package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type claudeImagePartialWriter struct {
	gin.ResponseWriter
	afterImage bool
}

func (w claudeImagePartialWriter) Write(p []byte) (int, error) {
	n := strings.Index(string(p), "data:image/png;base64,")
	if n < 0 {
		return w.ResponseWriter.Write(p)
	}
	if w.afterImage {
		end := strings.Index(string(p[n:]), "\n\n")
		if end >= 0 {
			n += end + 2
		}
	}
	written, _ := w.ResponseWriter.Write(p[:n])
	return written, errors.New("client disconnected")
}

// TestClaudeNanoImageDeliveryQuota exercises provider conversion and physical settlement.
func TestClaudeNanoImageDeliveryQuota(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		stream, fail, after, stale bool
		images                     int
		charge                     int64
	}{
		{"json_success", false, false, false, false, 1, 16802},
		{"json_retry_after_incomplete", false, false, false, true, 1, 16802},
		{"json_disconnect", false, true, false, false, 0, 1122},
		{"stream_success", true, false, false, false, 1, 16802},
		{"stream_disconnect_before_image", true, true, false, false, 0, 1122},
		{"stream_disconnect_after_image", true, true, true, false, 1, 17922},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 100000
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 20
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			payload := `{"candidates":[{"content":{"parts":[{"text":"hello"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1121,"totalTokenCount":1122,"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":1},{"modality":"IMAGE","tokenCount":1120}]}}`
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: "+payload+"\n\ndata: [DONE]\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, payload)
			}))
			t.Cleanup(srv.Close)
			old := client.HTTPClient
			client.HTTPClient = srv.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			body := `{"model":"alias","max_tokens":1,"messages":[{"role":"user","content":"draw"}]`
			if tc.stream {
				body += `,"stream":true`
			}
			body += `}`
			cfg := &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: 0.0336, DefaultSize: "1024x1024", SizeMultipliers: map[string]float64{"1024x1024": 1}}}
			c, rec, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", "/v1/messages", body, srv.URL, balance, 1, false, cfg)
			if tc.stale {
				c.Set(ctxkey.GeminiOutputIncomplete, true)
			}
			if tc.fail {
				c.Writer = claudeImagePartialWriter{ResponseWriter: c.Writer, afterImage: tc.after}
			}
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			if tc.fail {
				require.NotNil(t, apiErr)
			} else {
				require.Nil(t, apiErr)
				require.Contains(t, rec.Body.String(), "data:image/png;base64,aW1hZ2U=")
			}
			require.Equal(t, tc.images, c.GetInt(ctxkey.OutputImageCount))
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
		})
	}
}

type claudeImageShortWriter struct {
	gin.ResponseWriter
	limit int
}

func (w claudeImageShortWriter) Write(p []byte) (int, error) {
	return w.ResponseWriter.Write(p[:min(len(p), w.limit)])
}

// TestClaudeImageShortWriteCountsOnlyCompleteBlocks rejects a silent short write.
func TestClaudeImageShortWriteCountsOnlyCompleteBlocks(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Writer = claudeImageShortWriter{ResponseWriter: c.Writer, limit: 7}
	receipt := &relaymodel.ClaudeImageReceipt{ImageEndOffsets: []int{4, 8}}
	err := writeClaudeGeminiOutput(c, []byte("1234567890"), receipt)
	require.ErrorIs(t, err, io.ErrShortWrite)
	require.Equal(t, 1, c.GetInt(ctxkey.OutputImageCount))
	require.True(t, c.GetBool(ctxkey.GeminiOutputIncomplete))
}

// TestClaudeImageReceiptResetOnRetry prevents one attempt from billing another attempt.
func TestClaudeImageReceiptResetOnRetry(t *testing.T) {
	xaiVideoSetup(t, 100000, false)
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusInternalServerError) }))
	t.Cleanup(server.Close)
	c, _, _ := protocolContext(t, channeltype.Gemini, "gemini-nano-banana-2.1", "/v1/messages", `{}`, server.URL, 100000, 1, false, &dbmodel.ModelConfigLocal{Ratio: 1})
	c.Set(ctxkey.ClaudeGeminiImageReceipt, &relaymodel.ClaudeImageReceipt{CandidateImageTokens: 1120, ImageEndOffsets: []int{5}})
	failure := RelayClaudeMessagesHelper(c)
	require.NotNil(t, failure)
	require.Zero(t, calls)
	value, exists := c.Get(ctxkey.ClaudeGeminiImageReceipt)
	require.True(t, !exists || value == nil, "a later attempt must not inherit prior delivery evidence")
}
