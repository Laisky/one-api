package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// TestAudioRejectedOrpheusContract proves incompatible formats and oversized
// input fail before either upstream dispatch or creation of a billing record.
func TestAudioRejectedOrpheusContract(t *testing.T) {
	for _, input := range []string{`"input":"Hello","response_format":"mp3"`, `"input":"` + strings.Repeat("é", 201) + `"`, `"input":"hello","extra_body":{"response_format":"mp3"}`} {
		const balance = int64(1000)
		xaiVideoSetup(t, balance, false)
		c, _, id := protocolContext(t, channeltype.Groq, "canopylabs/orpheus-v1-english", "/v1/audio/speech", `{"model":"alias","voice":"troy",`+input+`}`, "https://unreachable.invalid", balance, 1, false, nil)
		apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
		require.NotNil(t, apiErr)
		require.Equal(t, 400, apiErr.StatusCode)
		require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
		drainCriticalTasks(t)
		require.Equal(t, balance, reloadUserQuota(t))
		var rows int64
		require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&rows).Error)
		require.Zero(t, rows)
	}
}

// TestAudioRejectedAmbiguousTranscription verifies the provider cannot select a
// remote URL while the gateway meters a different, cheaper local audio file.
func TestAudioRejectedAmbiguousTranscription(t *testing.T) {
	for _, remoteField := range []string{"url", "file_url", "audio_url"} {
		const balance = int64(10000)
		xaiVideoSetup(t, balance, false)
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		require.NoError(t, form.WriteField("model", "alias"))
		require.NoError(t, form.WriteField(remoteField, "https://example.com/large.wav"))
		file, err := form.CreateFormFile("file", "short.wav")
		require.NoError(t, err)
		_, err = io.WriteString(file, "not-the-remote-audio")
		require.NoError(t, err)
		require.NoError(t, form.Close())
		c, _, _ := protocolContext(t, channeltype.Groq, "whisper-large-v3", "/v1/audio/transcriptions", body.String(), "https://unreachable.invalid", balance, 1, false, nil)
		c.Request.Header.Set("Content-Type", form.FormDataContentType())
		apiErr := RelayAudioHelper(c, relaymode.AudioTranscription)
		require.NotNil(t, apiErr)
		require.Equal(t, 400, apiErr.StatusCode)
		require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
		drainCriticalTasks(t)
		require.Equal(t, balance, reloadUserQuota(t))
	}
}

// TestAudioRejectedSuccessEnvelope verifies false-success HTTP responses never
// charge users: empty audio, HTML and JSON error/missing-data receipts refund.
func TestAudioRejectedSuccessEnvelope(t *testing.T) {
	for _, tc := range []struct{ contentType, body string }{
		{"audio/mpeg", ""}, {"text/html", "<html>error</html>"},
		{"application/json", `{"error":{"message":"rejected"}}`},
		{"application/json", `{"code":20012,"message":"rejected"}`},
		{"application/json", `null`},
	} {
		const balance = int64(1000)
		xaiVideoSetup(t, balance, false)
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", tc.contentType)
			_, _ = io.WriteString(w, tc.body)
		}))
		previous := client.HTTPClient
		client.HTTPClient = server.Client()
		c, _, id := protocolContext(t, channeltype.OpenAI, "tts-1", "/v1/audio/speech", `{"model":"alias","input":"hello","voice":"alloy"}`, server.URL, balance, 1, false, nil)
		apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
		client.HTTPClient = previous
		server.Close()
		require.NotNil(t, apiErr)
		drainCriticalTasks(t)
		require.Equal(t, balance, reloadUserQuota(t))
		require.Zero(t, requestCostQuota(t, id))
	}
}
