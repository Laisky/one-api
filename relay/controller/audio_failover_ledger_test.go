package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// audioFailoverReceipt contains the HTTP request observed by the local provider.
// Parsing errors are returned to the test goroutine instead of aborting handlers.
type audioFailoverReceipt struct {
	fields                  map[string]any
	file                    []byte
	contentType, path, auth string
	length                  int64
	err                     error
}

// readAudioFailoverReceipt decodes one JSON or multipart upstream request and
// returns its model, extensions, upload bytes and transport metadata.
func readAudioFailoverReceipt(r *http.Request, multipart bool) audioFailoverReceipt {
	got := audioFailoverReceipt{contentType: r.Header.Get("Content-Type"), path: r.URL.Path, auth: r.Header.Get("Authorization"), length: r.ContentLength}
	if !multipart {
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		got.err = decoder.Decode(&got.fields)
		return got
	}
	got.err = r.ParseMultipartForm(1 << 20)
	if got.err != nil {
		return got
	}
	defer r.MultipartForm.RemoveAll()
	got.fields = map[string]any{"model": r.FormValue("model"), "language": r.FormValue("language")}
	f, _, err := r.FormFile("file")
	if err != nil {
		got.err = err
		return got
	}
	defer f.Close()
	got.file, got.err = io.ReadAll(f)
	return got
}

// TestPR421AudioFailoverLedger exercises two real HTTP attempts on the same Gin
// request using the production retry reset. It proves provider remapping, client
// body/boundary immutability, failure refunds and the accepted channel's exact
// user, token, request-cost and consume-log charges, including free overrides.
func TestPR421AudioFailoverLedger(t *testing.T) {
	for _, tc := range []struct {
		name, firstModel, secondModel string
		firstChannel, secondChannel   int
		multipart, trusted, unlimited bool
		group                         float64
		charge                        int64
		local                         *model.ModelConfigLocal
	}{
		{name: "mistral_to_groq", firstChannel: channeltype.Mistral, firstModel: "voxtral-mini-tts-2603", secondChannel: channeltype.Groq, secondModel: "canopylabs/orpheus-v1-english", group: 1, charge: 55},
		{name: "groq_to_mistral", firstChannel: channeltype.Groq, firstModel: "canopylabs/orpheus-v1-english", secondChannel: channeltype.Mistral, secondModel: "voxtral-mini-tts-2603", group: 1, charge: 40},
		{name: "mistral_to_openai", firstChannel: channeltype.Mistral, firstModel: "voxtral-mini-tts-2603", secondChannel: channeltype.OpenAI, secondModel: "tts-1", group: 1, charge: 38},
		{name: "trusted_override", firstChannel: channeltype.Mistral, firstModel: "voxtral-mini-tts-2603", secondChannel: channeltype.Groq, secondModel: "canopylabs/orpheus-v1-english", group: 1.5, charge: 77, trusted: true, local: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1e6, InputPriceUsd: 20.5}}},
		{name: "unlimited", firstChannel: channeltype.Mistral, firstModel: "voxtral-mini-tts-2603", secondChannel: channeltype.Groq, secondModel: "canopylabs/orpheus-v1-english", group: 1, charge: 55, unlimited: true},
		{name: "free_override", firstChannel: channeltype.Mistral, firstModel: "voxtral-mini-tts-2603", secondChannel: channeltype.Groq, secondModel: "canopylabs/orpheus-v1-english", group: 1, local: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1e6}}},
		{name: "multipart_groq_to_mistral", firstChannel: channeltype.Groq, firstModel: "whisper-large-v3", secondChannel: channeltype.Mistral, secondModel: "voxtral-mini-2602", group: 1, charge: 50, multipart: true},
		{name: "multipart_mistral_to_groq", firstChannel: channeltype.Mistral, firstModel: "voxtral-mini-2602", secondChannel: channeltype.Groq, secondModel: "whisper-large-v3", group: 1, charge: 155, multipart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			balance := int64(1000)
			if tc.trusted {
				balance = 1000000
			}
			xaiVideoSetup(t, balance, tc.unlimited)
			path, mode := "/v1/audio/speech", relaymode.AudioSpeech
			body := []byte(`{"model":"alias","input":"hello","voice":"troy","response_format":"wav","extra_body":{"seed":9007199254740993,"normalize":false}}`)
			contentType := "application/json"
			if tc.multipart {
				path, mode = "/v1/audio/transcriptions", relaymode.AudioTranscription
				body, contentType = silentAudioForm(t, 2)
			}
			seen := make(chan audioFailoverReceipt, 2)
			var calls atomic.Int32
			sound := silentWAV(t, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := calls.Add(1)
				got := readAudioFailoverReceipt(r, tc.multipart)
				select {
				case seen <- got:
				default:
				}
				if attempt == 1 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(503)
					_, _ = io.WriteString(w, `{"error":{"type":"overloaded","message":"retry another channel"}}`)
					return
				}
				if tc.multipart {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"text":"transcribed"}`)
					return
				}
				if tc.secondChannel == channeltype.Mistral {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"audio_data":"`+base64.StdEncoding.EncodeToString(sound)+`"}`)
					return
				}
				w.Header().Set("Content-Type", "audio/wav")
				_, _ = w.Write(sound)
			}))
			defer upstream.Close()
			originalClient := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = originalClient }()
			c, w, id := protocolContext(t, tc.firstChannel, tc.firstModel, path, string(body), upstream.URL, balance, 1, tc.unlimited, nil)
			c.Request.Header.Set("Content-Type", contentType)
			c.Set(ctxkey.ContentType, contentType)
			length := c.Request.ContentLength
			firstErr := RelayAudioHelper(c, mode)
			drainCriticalTasks(t)
			require.NotNil(t, firstErr)
			require.Equal(t, http.StatusServiceUnavailable, firstErr.StatusCode)
			require.Equal(t, balance, reloadUserQuota(t), "failed first attempt must be refunded")
			require.Zero(t, requestCostQuota(t, id))
			ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
			second := &model.Channel{Type: tc.secondChannel, Name: "second-audio-provider", Key: "second-fixture-key"}
			if tc.local != nil {
				require.NoError(t, second.SetModelPriceConfigs(map[string]model.ModelConfigLocal{tc.secondModel: *tc.local}))
			}
			require.NoError(t, model.DB.Create(second).Error)
			t.Cleanup(func() { drainCriticalTasks(t); require.NoError(t, model.DB.Delete(second).Error) })
			c.Set(ctxkey.ChannelId, second.Id)
			c.Set(ctxkey.Channel, tc.secondChannel)
			c.Set(ctxkey.ChannelModel, second)
			c.Set(ctxkey.ModelMapping, map[string]string{"alias": tc.secondModel})
			c.Set(ctxkey.ChannelRatio, tc.group)
			c.Request.Header.Set("Authorization", "Bearer second-fixture-key")
			// This is the same body reconstruction as controller.Relay's retry loop.
			original, err := common.GetRequestBody(c)
			require.NoError(t, err)
			c.Request.Body = io.NopCloser(bytes.NewReader(original))
			secondErr := RelayAudioHelper(c, mode)
			drainCriticalTasks(t)
			require.Nil(t, secondErr, "the second channel must receive a fresh standard request")
			require.Equal(t, int32(2), calls.Load(), "one rejected and one accepted upstream attempt")
			for i, expectedModel := range []string{tc.firstModel, tc.secondModel} {
				var got audioFailoverReceipt
				select {
				case got = <-seen:
				case <-time.After(5 * time.Second):
					t.Fatal("upstream attempt not observed")
				}
				require.NoError(t, got.err)
				require.Equal(t, expectedModel, got.fields["model"])
				require.Equal(t, path, got.path)
				require.Positive(t, got.length)
				expectedChannel := tc.firstChannel
				expectedAuth := "Bearer upstream-fixture-key"
				if i == 1 {
					expectedChannel = tc.secondChannel
					expectedAuth = "Bearer second-fixture-key"
				}
				require.Equal(t, expectedAuth, got.auth)
				if tc.multipart {
					require.Equal(t, silentWAV(t, 2), got.file)
					require.Equal(t, "en", got.fields["language"])
					require.NotEqual(t, contentType, got.contentType)
				} else {
					require.Equal(t, json.Number("9007199254740993"), got.fields["seed"])
					require.Equal(t, false, got.fields["normalize"])
					require.Equal(t, "hello", got.fields["input"])
					require.NotContains(t, got.fields, "extra_body")
					if expectedChannel == channeltype.Mistral {
						require.Equal(t, "troy", got.fields["voice_id"])
						require.NotContains(t, got.fields, "voice")
					} else {
						require.Equal(t, "troy", got.fields["voice"])
						require.NotContains(t, got.fields, "voice_id")
					}
				}
			}
			cached, err := common.GetRequestBody(c)
			require.NoError(t, err)
			require.Equal(t, body, cached, "provider normalization must not replace client bytes")
			require.Equal(t, contentType, c.GetHeader("Content-Type"))
			require.Equal(t, contentType, c.GetString(ctxkey.ContentType))
			require.Equal(t, length, c.Request.ContentLength)
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			require.Equal(t, tc.charge, requestCostQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if tc.unlimited {
				require.Zero(t, token.UsedQuota)
				require.Equal(t, balance, token.RemainQuota)
			} else {
				require.Equal(t, tc.charge, token.UsedQuota)
				require.Equal(t, balance-tc.charge, token.RemainQuota)
			}
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Find(&logs).Error)
			var sum int
			for _, entry := range logs {
				sum += entry.Quota
				require.Equal(t, "alias", entry.ModelName)
			}
			require.EqualValues(t, tc.charge, sum)
			require.Equal(t, "alias", metalib.GetByContext(c).OriginModelName)
			require.Equal(t, tc.secondModel, metalib.GetByContext(c).ActualModelName)
			if tc.multipart {
				require.JSONEq(t, `{"text":"transcribed"}`, w.Body.String())
			} else {
				require.Equal(t, sound, w.Body.Bytes())
			}
		})
	}
}
