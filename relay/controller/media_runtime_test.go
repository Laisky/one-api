package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMediaRuntimeCorpusBilling checks actual multipart metering and ledger settlement.
func TestMediaRuntimeCorpusBilling(t *testing.T) {
	if os.Getenv("ONEAPI_MEDIA_RUNTIME") != "1" {
		t.Skip("set ONEAPI_MEDIA_RUNTIME=1 for explicit image qualification")
	}
	root := os.Getenv("ONEAPI_MEDIA_CORPUS_DIR")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	require.NoError(t, err)
	var corpus struct {
		Cases []struct {
			Name    string `json:"name"`
			Path    string `json:"path"`
			Success bool   `json:"success"`
			Quota   int64  `json:"quota"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(data, &corpus))
	require.GreaterOrEqual(t, len(corpus.Cases), 20)
	for _, fixture := range corpus.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			const balance = int64(100000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			observed := make(chan struct {
				model string
				err   error
			}, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				parseErr := r.ParseMultipartForm(1 << 20)
				if r.MultipartForm != nil {
					defer r.MultipartForm.RemoveAll()
				}
				w.Header().Set("Content-Type", "application/json")
				_, writeErr := io.WriteString(w, `{"text":"synthetic transcript"}`)
				if parseErr == nil {
					parseErr = writeErr
				}
				observed <- struct {
					model string
					err   error
				}{r.FormValue("model"), parseErr}
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("model", "alias"))
			part, err := writer.CreateFormFile("file", filepath.Base(fixture.Path))
			require.NoError(t, err)
			content, err := os.ReadFile(filepath.Join(root, fixture.Path))
			require.NoError(t, err)
			_, err = part.Write(content)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			config := &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{
				InputUnit: "seconds", InputPriceQuantity: 1, InputPriceUsd: 0.001,
			}}
			c, _, id := protocolContext(t, channeltype.OpenAI, "whisper-1",
				"/v1/audio/transcriptions", body.String(), server.URL, balance, 1, false, config)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			c.Set(ctxkey.ContentType, writer.FormDataContentType())
			apiErr := RelayAudioHelper(c, relaymode.AudioTranscription)
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if !fixture.Success {
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				require.Zero(t, calls.Load())
				require.Equal(t, balance, reloadUserQuota(t))
				require.Zero(t, token.UsedQuota)
				var costs int64
				require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costs).Error)
				require.Zero(t, costs, "unprobeable input must not create a billed request")
				return
			}
			require.Nil(t, apiErr)
			require.Equal(t, int32(1), calls.Load())
			seen := <-observed
			require.NoError(t, seen.err)
			require.Equal(t, "whisper-1", seen.model)
			require.Equal(t, balance-fixture.Quota, reloadUserQuota(t))
			require.Equal(t, fixture.Quota, requestCostQuota(t, id))
			require.Equal(t, fixture.Quota, token.UsedQuota)
			require.Equal(t, balance-fixture.Quota, token.RemainQuota)
			t.Logf("fixture=%s quota=%d", fixture.Name, fixture.Quota)
		})
	}
}

// TestMediaRuntimeGeminiEncodingLedger checks actual compressed speech and accepted billing.
func TestMediaRuntimeGeminiEncodingLedger(t *testing.T) {
	if os.Getenv("ONEAPI_MEDIA_RUNTIME") != "1" {
		t.Skip("set ONEAPI_MEDIA_RUNTIME=1 for explicit image qualification")
	}
	for _, tc := range []struct {
		name, format string
		speed        float64
		stream       bool
	}{
		{name: "default_mp3", speed: 2},
		{name: "buffered_opus_sse", format: "opus", speed: 0.25, stream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10000)
			xaiVideoSetup(t, balance, false)
			payload := map[string]any{
				"candidates": []any{map[string]any{
					"content": map[string]any{"parts": []any{map[string]any{
						"inlineData": map[string]any{
							"mimeType": "audio/L16;rate=24000",
							"data":     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1, 2}, 1920)),
						},
					}}},
					"finishReason": "STOP",
				}},
				"usageMetadata": map[string]int{"promptTokenCount": 12, "candidatesTokenCount": 2, "cachedContentTokenCount": 4},
			}
			upstream, err := json.Marshal(payload)
			require.NoError(t, err)
			var calls atomic.Int32
			written := make(chan error, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				body := upstream
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					body = append(append([]byte("data: "), upstream...), []byte("\n\n")...)
				}
				_, err := w.Write(body)
				written <- err
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			request := map[string]any{
				"model": "alias", "input": "Synthetic runtime speech.", "voice": "synthetic-voice",
				"speed": tc.speed, "gemini": map[string]int{"max_output_tokens": 10},
			}
			if tc.format != "" {
				request["response_format"] = tc.format
			}
			if tc.stream {
				request["stream_format"] = "sse"
			}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			c, w, id := protocolContext(t, channeltype.Gemini, "gemini-3.8-flash-tts",
				"/v1/audio/speech", string(body), server.URL, balance, 1, false, nil)
			meta.GetByContext(c).StartTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
			apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.Equal(t, int32(1), calls.Load())
			require.NoError(t, <-written)
			require.True(t, c.GetBool(adaptor.AudioReceiptAcceptedKey))
			require.Equal(t, balance-12, reloadUserQuota(t))
			require.Equal(t, int64(12), requestCostQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, int64(12), token.UsedQuota)
			require.NotEmpty(t, w.Body.Bytes())
			if tc.stream {
				require.Contains(t, w.Body.String(), "speech.audio.delta")
				require.Contains(t, w.Body.String(), "speech.audio.done")
			} else {
				require.Equal(t, "audio/mpeg", w.Header().Get("Content-Type"))
			}
		})
	}
}
