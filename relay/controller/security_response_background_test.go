package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityResponseBackgroundAdmission proves native background jobs cannot dispatch or change balances before durable terminal settlement exists.
func TestSecurityResponseBackgroundAdmission(t *testing.T) {
	for _, streaming := range []string{"false", "true"} {
		t.Run("stream="+streaming, func(t *testing.T) {
			securityAdmissionSetup(t, 1_000_000)
			before := fallbackUserQuota(t)
			var beforeToken model.Token
			require.NoError(t, model.DB.First(&beforeToken, fallbackTokenID).Error)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"id":"resp_queued","object":"response","status":"queued","output":[],"usage":null}`))
				if err != nil {
					t.Errorf("write queued fixture response: %v", err)
				}
			}))
			defer upstream.Close()
			c := setupResponseStateBillingContext(t, httptest.NewRecorder(), `{"model":"gpt-4o-mini","input":"hello","background":true,"stream":`+streaming+`}`)
			c.Set(ctxkey.Channel, channeltype.OpenAI)
			c.Set(ctxkey.ChannelId, fallbackOpenAIChannelID)
			c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackOpenAIChannelID, Type: channeltype.OpenAI})
			c.Set(ctxkey.BaseURL, upstream.URL+"/api.openai.com")
			err := RelayResponseAPIHelper(c)
			drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, graceful.Drain(drain))
			require.NotNil(t, err, "queued jobs must be rejected before the initial prompt-only receipt can settle their hold")
			require.Equal(t, http.StatusBadRequest, err.StatusCode)
			require.Equal(t, "background_not_supported", err.Code)
			require.Zero(t, calls.Load())
			require.Equal(t, before, fallbackUserQuota(t))
			var afterToken model.Token
			require.NoError(t, model.DB.First(&afterToken, fallbackTokenID).Error)
			require.Equal(t, beforeToken.RemainQuota, afterToken.RemainQuota)
			require.Equal(t, beforeToken.UsedQuota, afterToken.UsedQuota)
		})
	}
}

// TestSecurityResponseForegroundValidation keeps omitted and false background flags valid for synchronous execution.
func TestSecurityResponseForegroundValidation(t *testing.T) {
	for _, field := range []string{"", `,"background":false`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-4o-mini","input":"hello"`+field+`}`))
		c.Request.Header.Set("Content-Type", "application/json")
		req, err := getAndValidateResponseAPIRequest(c)
		require.NoError(t, err)
		require.NotNil(t, req)
	}
}
