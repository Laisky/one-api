package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// protocolContext prepares real channel mapping and price overrides on the shared
// SQLite fixture; the HTTP body and provider credentials remain observable.
func protocolContext(t *testing.T, channel int, actual, path, body, base string, balance int64, group float64, unlimited bool, local *model.ModelConfigLocal) (*gin.Context, *httptest.ResponseRecorder, string) {
	t.Helper()
	c, w, id := xaiVideoContext(t, http.MethodPost, path, body, base, balance, group, unlimited, nil)
	ch := &model.Channel{Id: fallbackChannelID, Type: channel, Name: "protocol-fixture", Key: "upstream-fixture-key"}
	if local != nil {
		require.NoError(t, ch.SetModelPriceConfigs(map[string]model.ModelConfigLocal{actual: *local}))
	}
	c.Set(ctxkey.Channel, channel)
	c.Set(ctxkey.ChannelModel, ch)
	c.Set(ctxkey.ModelMapping, map[string]string{"alias": actual})
	c.Request.Header.Set("Authorization", "Bearer upstream-fixture-key")
	return c, w, id
}

// TestProtocolAuditAudioHTTP verifies input units, actual mapped HTTP requests,
// binary output, quota admission, exact user/token/request ledgers and refunds.
func TestProtocolAuditAudioHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, actual, text, extra string
		channel, status           int
		charge                    int64
		group                     float64
		low, unlimited, writeFail bool
		override                  *model.ModelConfigLocal
	}{
		{name: "mistral_chars", actual: "voxtral-mini-tts-2603", text: "你é🙂", channel: channeltype.Mistral, status: 200, charge: 24, group: 1},
		{name: "mistral_legacy_alias", actual: "voxtral-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 40, group: 1},
		{name: "mistral_latest", actual: "voxtral-mini-tts-latest", text: "hello", channel: channeltype.Mistral, status: 200, charge: 40, group: 1},
		{name: "siliconflow_bytes", actual: "FunAudioLLM/CosyVoice2-0.5B", text: "你é🙂", channel: channeltype.SiliconFlow, status: 200, charge: 33, group: 1},
		{name: "groq_chars", actual: "canopylabs/orpheus-v1-english", text: "你é🙂", channel: channeltype.Groq, status: 200, charge: 33, group: 1},
		{name: "groq_arabic", actual: "canopylabs/orpheus-arabic-saudi", text: "مرحبا", channel: channeltype.Groq, status: 200, charge: 100, group: 1},
		{name: "legacy_character_tariff", actual: "tts-1", text: "你é🙂", channel: channeltype.OpenAI, status: 200, charge: 23, group: 1},
		{name: "group", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 60, group: 1.5},
		{name: "free_group", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 0, group: 0},
		{name: "unlimited_token", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 40, group: 1, unlimited: true},
		{name: "explicit_rate", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 25, group: 1, override: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 10}}},
		{name: "explicit_free", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 0, group: 1, override: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1e6}}},
		{name: "legacy_alias_rate", actual: "voxtral-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 25, group: 1, override: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 10}}},
		{name: "metadata_only", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 40, group: 1, override: &model.ModelConfigLocal{MaxTokens: 100}},
		{name: "ratio_override", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 10, group: 1, override: &model.ModelConfigLocal{Ratio: 2}},
		{name: "insufficient", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, group: 1, low: true},
		{name: "refused", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 403, group: 1},
		{name: "limited", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 429, group: 1},
		{name: "overloaded", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 503, group: 1},
		{name: "write_failure", actual: "voxtral-mini-tts-2603", text: "hello", channel: channeltype.Mistral, status: 200, charge: 40, group: 1, writeFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			balance := int64(1000)
			if tc.low {
				balance = 1
			}
			xaiVideoSetup(t, balance, tc.unlimited)
			var calls atomic.Int32
			obs := make(chan map[string]any, 1)
			sound := silentWAV(t, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var wire map[string]any
				err := json.NewDecoder(r.Body).Decode(&wire)
				obs <- map[string]any{"body": wire, "error": err, "path": r.URL.Path, "authorization": r.Header.Get("Authorization")}
				if tc.status != 200 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, `{"error":{"message":"refused","type":"invalid_request_error"}}`)
					return
				}
				if tc.channel == channeltype.Mistral {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"audio_data":"`+base64.StdEncoding.EncodeToString(sound)+`"}`)
				} else {
					w.Header().Set("Content-Type", "audio/wav")
					_, _ = w.Write(sound)
				}
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			format := "wav"
			voice := "test-voice-id"
			switch tc.actual {
			case "canopylabs/orpheus-v1-english":
				voice = "troy"
			case "canopylabs/orpheus-arabic-saudi":
				voice = "fahad"
			case "FunAudioLLM/CosyVoice2-0.5B":
				voice = "FunAudioLLM/CosyVoice2-0.5B:alex"
			case "tts-1":
				voice = "alloy"
			}
			input, _ := json.Marshal(tc.text)
			body := `{"model":"alias","input":` + string(input) + `,"voice":"` + voice + `","response_format":"` + format + `","extra_body":{"seed":9007199254740993,"normalize":false}}`
			c, w, id := protocolContext(t, tc.channel, tc.actual, "/v1/audio/speech", body, server.URL, balance, tc.group, tc.unlimited, tc.override)
			if tc.writeFail {
				c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
			}
			apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
			drainCriticalTasks(t)
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if tc.unlimited {
				require.Zero(t, token.UsedQuota, "unlimited token counters remain unchanged")
				require.Equal(t, balance, token.RemainQuota)
			} else {
				require.Equal(t, tc.charge, token.UsedQuota)
				require.Equal(t, balance-tc.charge, token.RemainQuota)
			}
			if tc.low {
				var costs int64
				require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costs).Error)
				require.Zero(t, costs, "admission refusal must not create a billed request")
				require.NotNil(t, apiErr)
				require.Zero(t, calls.Load())
				return
			}
			require.Equal(t, tc.charge, requestCostQuota(t, id))
			require.Equal(t, int32(1), calls.Load())
			observed := <-obs
			require.Nil(t, observed["error"])
			wire := observed["body"].(map[string]any)
			expected := tc.actual
			if expected == "voxtral-tts-2603" {
				expected = "voxtral-mini-tts-2603"
			}
			require.Equal(t, expected, wire["model"])
			require.Equal(t, tc.text, wire["input"])
			require.NotContains(t, wire, "extra_body")
			require.Equal(t, false, wire["normalize"])
			require.Equal(t, "Bearer upstream-fixture-key", observed["authorization"])
			if tc.channel == channeltype.Mistral {
				require.NotContains(t, wire, "voice")
				require.Equal(t, voice, wire["voice_id"])
			}
			if tc.status != 200 || tc.writeFail {
				require.NotNil(t, apiErr)
			} else {
				require.Nil(t, apiErr)
				require.Equal(t, sound, w.Body.Bytes())
				require.Equal(t, "audio/wav", w.Header().Get("Content-Type"))
			}
		})
	}
}

// silentWAV returns valid PCM WAV bytes with the requested physical duration.
func silentWAV(t *testing.T, seconds int) []byte {
	t.Helper()
	var wav bytes.Buffer
	samples := seconds * 8000
	for _, value := range []any{[]byte("RIFF"), uint32(36 + samples*2), []byte("WAVEfmt "), uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16), []byte("data"), uint32(samples * 2), make([]byte, samples*2)} {
		require.NoError(t, binary.Write(&wav, binary.LittleEndian, value))
	}
	return wav.Bytes()
}

// silentAudioForm generates a physical PCM WAV with a known duration; ffprobe,
// rather than a mocked duration function, measures the uploaded samples.
func silentAudioForm(t *testing.T, seconds int) ([]byte, string) {
	t.Helper()
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	require.NoError(t, writer.WriteField("model", "alias"))
	require.NoError(t, writer.WriteField("language", "en"))
	file, err := writer.CreateFormFile("file", "speech.wav")
	require.NoError(t, err)
	_, err = file.Write(silentWAV(t, seconds))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return form.Bytes(), writer.FormDataContentType()
}

// TestProtocolAuditTranscriptionHTTP verifies exact per-duration charging,
// minimum fees and model mapping in the actual multipart body.
func TestProtocolAuditTranscriptionHTTP(t *testing.T) {
	_, err := exec.LookPath("ffprobe")
	require.NoError(t, err, "ffmpeg is required for physical audio metering tests")
	for _, tc := range []struct {
		name, actual     string
		channel, seconds int
		charge           int64
	}{
		{"mistral", "voxtral-mini-2602", channeltype.Mistral, 2, 50},
		{"mistral_legacy", "voxtral-mini-transcribe-2602", channeltype.Mistral, 2, 50},
		{"whisper_minimum", "whisper-large-v3", channeltype.Groq, 1, 155},
		{"turbo_minimum", "whisper-large-v3-turbo", channeltype.Groq, 2, 56},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10000)
			xaiVideoSetup(t, balance, false)
			seen := make(chan string, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					seen <- err.Error()
				} else {
					seen <- r.FormValue("model")
				}
				if r.MultipartForm != nil {
					defer r.MultipartForm.RemoveAll()
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"text":"Hello world"}`)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			body, ctype := silentAudioForm(t, tc.seconds)
			c, w, id := protocolContext(t, tc.channel, tc.actual, "/v1/audio/transcriptions", string(body), server.URL, balance, 1, false, nil)
			c.Request.Header.Set("Content-Type", ctype)
			c.Set(ctxkey.ContentType, ctype)
			apiErr := RelayAudioHelper(c, relaymode.AudioTranscription)
			require.Nil(t, apiErr)
			drainCriticalTasks(t)
			expectedModel := tc.actual
			if expectedModel == "voxtral-mini-transcribe-2602" {
				expectedModel = "voxtral-mini-2602"
			}
			require.Equal(t, expectedModel, <-seen)
			require.JSONEq(t, `{"text":"Hello world"}`, w.Body.String())
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			require.Equal(t, tc.charge, requestCostQuota(t, id))
		})
	}
}

// TestAudioTariffArithmetic guards against repeating-hour conversions, Unicode
// byte confusion, zero prices, minimums, increments and invalid floating values.
func TestAudioTariffArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    int
		text    string
		seconds float64
		cfg     adaptor.AudioPricingConfig
		charge  int64
	}{
		{"one_hour", relaymode.AudioTranscription, "", 3600, adaptor.AudioPricingConfig{InputUnit: "seconds", InputPriceQuantity: 3600, InputPriceUsd: .111}, 55500},
		{"unicode_chars", relaymode.AudioSpeech, "你é🙂", 0, adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 16}, 24},
		{"unicode_bytes", relaymode.AudioSpeech, "你é🙂", 0, adaptor.AudioPricingConfig{InputUnit: "utf8_bytes", InputPriceQuantity: 1e6, InputPriceUsd: 7.15}, 33},
		{"fractional_increment", relaymode.AudioTranscription, "", .3, adaptor.AudioPricingConfig{InputUnit: "seconds", InputPriceQuantity: 1, InputPriceUsd: 1, BillingIncrementSeconds: .1}, 150000},
		{"round_increment", relaymode.AudioTranscription, "", 1.01, adaptor.AudioPricingConfig{InputUnit: "seconds", InputPriceQuantity: 1, InputPriceUsd: .01, BillingIncrementSeconds: 1}, 10000},
		{"free", relaymode.AudioSpeech, "paid text", 0, adaptor.AudioPricingConfig{InputUnit: "characters", InputPriceQuantity: 1e6}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, _, err := quoteAudioInput(tc.mode, tc.text, tc.seconds, 20, 100, 1, &tc.cfg, false)
			require.NoError(t, err)
			require.Equal(t, tc.charge, q)
		})
	}
	for _, v := range []float64{-1, math.NaN(), math.Inf(1), math.MaxFloat64} {
		_, err := decimalQuotaRate(1, v, 500000)
		require.Error(t, err)
	}
	// Rounding happens once after group scaling, including sub-quota prices.
	q, err := decimalQuotaRate(1e6, 1, 7.15, 500000, .5)
	require.NoError(t, err)
	require.Equal(t, int64(2), q)
}

// TestAudioRejectedInputHasNoSideEffect verifies validation precedes both quota
// admission and network dispatch, including protected extra_body fields.
func TestAudioRejectedInputHasNoSideEffect(t *testing.T) {
	for _, fields := range []string{
		`"input":""`, `"input":"hello","extra_body":{"input":"free replacement"}`,
		`"input":"hello","extra_body":{"model":"other"}`, `"input":"hello","stream":true`,
		`"input":"hello","speed":2`, `"input":"hello","response_format":"pcm"`,
		`"input":"hello","voice_id":"different"`,
	} {
		t.Run(fields, func(t *testing.T) {
			const balance = int64(1000)
			xaiVideoSetup(t, balance, false)
			c, _, _ := protocolContext(t, channeltype.Mistral, "voxtral-mini-tts-2603", "/v1/audio/speech", `{"model":"alias","voice":"voice",`+fields+`}`, "https://unreachable.invalid", balance, 1, false, nil)
			err := RelayAudioHelper(c, relaymode.AudioSpeech)
			require.NotNil(t, err)
			require.Equal(t, 400, err.StatusCode)
			drainCriticalTasks(t)
			require.Equal(t, balance, reloadUserQuota(t))
		})
	}
}
