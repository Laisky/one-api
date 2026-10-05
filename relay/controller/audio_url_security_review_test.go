package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// reviewAudioFailureTransport injects a failure through the real HTTP client.
type reviewAudioFailureTransport func(*http.Request) (*http.Response, error)

// RoundTrip delegates the fixture request without external network access.
func (f reviewAudioFailureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestReviewAudioURLPrivacy verifies real audio dispatch, retained accounting,
// URL diagnostics and error causes on the legacy non-adaptor request path.
func TestReviewAudioURLPrivacy(t *testing.T) {
	for _, scenario := range []string{"success", "transport_error", "invalid_url"} {
		t.Run(scenario, func(t *testing.T) {
			const secret = "fixture-audio-url-secret"
			const balance = int64(1000)
			xaiVideoSetup(t, balance, false)
			wire := make(chan map[string]any, 1)
			audio := silentWAV(t, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var body map[string]any
				err := json.NewDecoder(req.Body).Decode(&body)
				wire <- map[string]any{"body": body, "err": err, "path": req.URL.Path, "auth": req.Header.Get("Authorization")}
				w.Header().Set("Content-Type", "audio/wav")
				_, _ = w.Write(audio)
			}))
			defer upstream.Close()
			base, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			base.User = url.UserPassword("operator", secret)
			rawBase := base.String()
			if scenario == "invalid_url" {
				rawBase += "/%zz?credential=" + secret
			}
			oldClient := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = oldClient }()
			var sentURL string
			if scenario == "transport_error" {
				client.HTTPClient = &http.Client{Transport: reviewAudioFailureTransport(func(req *http.Request) (*http.Response, error) {
					sentURL = req.URL.String()
					if req.Body != nil {
						_, _ = io.Copy(io.Discard, req.Body)
						_ = req.Body.Close()
					}
					return nil, context.DeadlineExceeded
				})}
			}
			body := `{"model":"alias","input":"hello","voice":"alloy","response_format":"wav"}`
			price := &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 10}}
			c, w, _ := protocolContext(t, channeltype.OpenAI, "tts-1", "/v1/audio/speech", body, rawBase, balance, 1, false, price)
			core, observed := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewWithName("audio-url-review", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			gmw.SetLogger(c, lg)
			result := RelayAudioHelper(c, relaymode.AudioSpeech)
			drainCriticalTasks(t)
			if scenario == "success" {
				require.Nil(t, result)
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, audio, w.Body.Bytes())
				got := <-wire
				require.Nil(t, got["err"])
				require.Equal(t, "/v1/audio/speech", got["path"])
				require.Equal(t, "Bearer upstream-fixture-key", got["auth"])
				require.Equal(t, "hello", got["body"].(map[string]any)["input"])
				require.Equal(t, balance-25, reloadUserQuota(t))
			} else {
				require.NotNil(t, result)
				require.Equal(t, balance, reloadUserQuota(t), "failed dispatch must retain the existing refund behavior")
				require.Empty(t, wire)
				if scenario == "transport_error" {
					require.Contains(t, sentURL, secret, "only the error copy may be sanitized")
					require.True(t, errors.Is(result.RawError, context.DeadlineExceeded))
				}
				require.NotContains(t, result.Message, secret)
				require.NotContains(t, result.RawError.Error(), secret)
			}
			if scenario != "invalid_url" {
				entries := observed.FilterMessage("sending audio request to upstream channel").All()
				require.Len(t, entries, 1, "retain the useful audio dispatch event")
				require.Equal(t, upstream.URL+"/v1/audio/speech", entries[0].ContextMap()["url"])
			}
			for _, entry := range observed.All() {
				require.NotContains(t, entry.Message, secret)
				for _, field := range entry.Context {
					require.NotContains(t, field.String, secret)
				}
			}
			require.Contains(t, rawBase, secret)
			require.True(t, strings.HasPrefix(rawBase, "http://"))
		})
	}
}
