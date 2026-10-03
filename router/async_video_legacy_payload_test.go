package router

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestAsyncVideoReplayPreservesLegacyPayloads verifies that merely including an
// idempotency key does not impose the new JSON-only/1-MiB contract on an existing
// synchronous provider. Both requests must reach the real provider unchanged.
func TestAsyncVideoReplayPreservesLegacyPayloads(t *testing.T) {
	for _, format := range []string{"form", "multipart", "large_json", "json_control"} {
		t.Run(format, func(t *testing.T) {
			var calls atomic.Int32
			var body bytes.Buffer
			contentType := "application/json"
			switch format {
			case "form":
				contentType = "application/x-www-form-urlencoded"
				body.WriteString("model=veo3-fast&duration=5&prompt=legacy")
			case "multipart":
				writer := multipart.NewWriter(&body)
				require.NoError(t, writer.WriteField("model", "veo3-fast"))
				require.NoError(t, writer.WriteField("duration", "5"))
				require.NoError(t, writer.WriteField("prompt", "legacy"))
				require.NoError(t, writer.Close())
				contentType = writer.FormDataContentType()
			case "large_json":
				fmt.Fprintf(&body, `{"model":"veo3-fast","duration":5,"prompt":%q}`, strings.Repeat("x", model.MaxAsyncTaskBody))
			default:
				body.WriteString(`{"model":"veo3-fast","duration":5}`)
			}
			original := bytes.Clone(body.Bytes())
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				received, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Equal(received, original) {
					http.Error(w, "changed legacy request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"created":123,"data":[{"url":"https://media.example/legacy.mp4"}]}`)
			}))
			defer upstream.Close()
			engine, key, userID, tokenID, _ := disconnectBillingRouter(t, upstream.URL)
			client.HTTPClient = upstream.Client()
			channel := model.Channel{Id: 919193}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{
				"veo3-fast": {PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 400}},
			}))
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
				"type": channeltype.OpenAICompatible, "model_configs": channel.ModelConfigs,
			}).Error)
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("quota", 1000000).Error)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("remain_quota", 1000000).Error)
			for _, idempotency := range []string{"", "legacy-request-key"} {
				request := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", bytes.NewReader(original))
				request.Header.Set("Content-Type", contentType)
				request.Header.Set("Authorization", "Bearer sk-"+key)
				request.Header.Set("Idempotency-Key", idempotency)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), "legacy.mp4")
				require.Empty(t, response.Header().Get("X-Async-Task-Id"))
			}
			drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, graceful.Drain(drain))
			require.EqualValues(t, 2, calls.Load())
			var tasks int64
			require.NoError(t, model.DB.Model(&model.AsyncTask{}).Count(&tasks).Error)
			require.Zero(t, tasks)
		})
	}
}
