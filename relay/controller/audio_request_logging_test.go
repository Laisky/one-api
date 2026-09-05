package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/relaymode"
)

// newObservedAudioContext returns a gin context whose request-scoped logger is captured.
func newObservedAudioContext(t *testing.T, req *http.Request) (*gin.Context, *observer.ObservedLogs) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("audio-request-logging-test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err, "create observer logger")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	gmw.SetLogger(c, lg)
	return c, observed
}

// audioParametersEntry returns the single "audio request parameters" record, failing the
// test when the number of records is not exactly one.
func audioParametersEntry(t *testing.T, observed *observer.ObservedLogs) observer.LoggedEntry {
	t.Helper()
	entries := observed.FilterMessage("audio request parameters").All()
	require.Len(t, entries, 1, "expected exactly one audio parameter log line")
	require.Equal(t, zapcore.DebugLevel, entries[0].Level,
		"request diagnostics belong at DEBUG")
	return entries[0]
}

// TestLogAudioRequestParameters_TranscriptionRecordsProtocolFields reproduces the
// production request that failed with
// "response_format 'vtt' is not compatible with model 'gpt-transcribe-api-ev3'":
// the caller's response_format was invisible in the gateway logs, because multipart
// bodies are excluded from the generic request logger. It must now be recorded.
func TestLogAudioRequestParameters_TranscriptionRecordsProtocolFields(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fw, err := writer.CreateFormFile("file", "meeting-recording.mp3")
	require.NoError(t, err, "create form file")
	_, err = fw.Write(bytes.Repeat([]byte("a"), 2048))
	require.NoError(t, err, "write audio bytes")
	require.NoError(t, writer.WriteField("model", "gpt-transcribe"), "write model")
	require.NoError(t, writer.WriteField("response_format", "vtt"), "write response_format")
	require.NoError(t, writer.WriteField("language", "en"), "write language")
	require.NoError(t, writer.WriteField("timestamp_granularities[]", "segment"), "write granularity")
	require.NoError(t, writer.Close(), "close writer")

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c, observed := newObservedAudioContext(t, req)

	// RelayAudioHelper parses the multipart form while extracting the model; mirror that.
	require.Equal(t, "gpt-transcribe", extractAudioModelFromMultipart(c))

	logAudioRequestParameters(c, relaymode.AudioTranscription, "gpt-transcribe", nil)

	fields := audioParametersEntry(t, observed).ContextMap()
	require.Equal(t, "gpt-transcribe", fields["audio_model"])
	require.EqualValues(t, relaymode.AudioTranscription, fields["relay_mode"])
	require.Equal(t, []any{"vtt"}, fields["response_format"],
		"the rejected parameter must be visible in one-api's own logs")
	require.Equal(t, []any{"en"}, fields["language"])
	require.Equal(t, []any{"segment"}, fields["timestamp_granularities[]"])
	require.Equal(t, ".mp3", fields["file_ext"])
	require.EqualValues(t, 2048, fields["file_bytes"])
	require.NotContains(t, fields, "model", "the raw model field is reported after mapping")
}

// TestLogAudioRequestParameters_NeverLogsCallerContent pins the privacy contract: a
// biasing prompt, keyword hints and speaker names are caller content, so only their size
// may be recorded. The audio filename is caller data too.
func TestLogAudioRequestParameters_NeverLogsCallerContent(t *testing.T) {
	t.Parallel()

	const promptSentinel = "sentinel-private-biasing-prompt"
	const speakerSentinel = "sentinel-private-speaker-name"
	const filenameSentinel = "sentinel-private-filename"

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fw, err := writer.CreateFormFile("file", filenameSentinel+".wav")
	require.NoError(t, err, "create form file")
	_, err = fw.Write([]byte("audio"))
	require.NoError(t, err, "write audio bytes")
	require.NoError(t, writer.WriteField("model", "whisper-1"), "write model")
	require.NoError(t, writer.WriteField("prompt", promptSentinel), "write prompt")
	require.NoError(t, writer.WriteField("known_speaker_names[]", speakerSentinel), "write speaker")
	require.NoError(t, writer.WriteField("x_unknown_switch", "sentinel-private-unknown-value"), "write unknown field")
	require.NoError(t, writer.Close(), "close writer")

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c, observed := newObservedAudioContext(t, req)
	require.Equal(t, "whisper-1", extractAudioModelFromMultipart(c))

	logAudioRequestParameters(c, relaymode.AudioTranscription, "whisper-1", nil)

	entry := audioParametersEntry(t, observed)
	rendered, err := json.Marshal(entry.ContextMap())
	require.NoError(t, err, "marshal log fields")

	require.NotContains(t, string(rendered), promptSentinel, "prompt content must never be logged")
	require.NotContains(t, string(rendered), speakerSentinel, "speaker names must never be logged")
	require.NotContains(t, string(rendered), filenameSentinel, "filenames must never be logged")
	require.NotContains(t, string(rendered), "sentinel-private-unknown-value",
		"values of unrecognized fields must never be logged")

	fields := entry.ContextMap()
	require.EqualValues(t, len(promptSentinel), fields["prompt_chars"],
		"prompt size stays observable for debugging")
	require.EqualValues(t, 1, fields["prompt_count"])
	require.Equal(t, []any{"x_unknown_switch"}, fields["unrecognized_fields"],
		"unknown field NAMES are reported so unsupported parameters are diagnosable")
	require.Equal(t, ".wav", fields["file_ext"])
}

// TestLogAudioRequestParameters_SpeechRecordsProtocolFieldsOnly covers the JSON speech
// path: voice and format are protocol switches, but the text to synthesize is content.
func TestLogAudioRequestParameters_SpeechRecordsProtocolFieldsOnly(t *testing.T) {
	t.Parallel()

	const inputSentinel = "sentinel-private-tts-input"

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/json")
	c, observed := newObservedAudioContext(t, req)

	logAudioRequestParameters(c, relaymode.AudioSpeech, "tts-1-hd", &openai.TextToSpeechRequest{
		Model:          "tts-1-hd",
		Input:          inputSentinel,
		Voice:          "alloy",
		Speed:          1.25,
		ResponseFormat: "opus",
	})

	entry := audioParametersEntry(t, observed)
	fields := entry.ContextMap()
	require.Equal(t, "tts-1-hd", fields["audio_model"])
	require.Equal(t, "opus", fields["response_format"])
	require.Equal(t, "alloy", fields["voice"])
	require.EqualValues(t, 1.25, fields["speed"])
	require.EqualValues(t, len(inputSentinel), fields["input_chars"])

	rendered, err := json.Marshal(fields)
	require.NoError(t, err, "marshal log fields")
	require.NotContains(t, string(rendered), inputSentinel, "synthesis input must never be logged")
}

// TestLogAudioRequestParameters_UnparsedFormIsReportedNotFatal guards the degenerate case:
// when the multipart form was never parsed the helper must still emit a line and must not
// panic, since it runs on the hot relay path.
func TestLogAudioRequestParameters_UnparsedFormIsReportedNotFatal(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(nil))
	c, observed := newObservedAudioContext(t, req)

	require.NotPanics(t, func() {
		logAudioRequestParameters(c, relaymode.AudioTranscription, "whisper-1", nil)
	})

	fields := audioParametersEntry(t, observed).ContextMap()
	require.Equal(t, "not parsed", fields["form_state"])
}

// TestLogAudioRequestParameters_BoundsHostileForm proves the log record stays bounded for
// a caller-controlled body: an oversized protocol value is truncated, repeated values are
// capped, and a flood of unknown field names is reported by count with a truncated sample.
func TestLogAudioRequestParameters_BoundsHostileForm(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fw, err := writer.CreateFormFile("file", "a.mp3")
	require.NoError(t, err, "create form file")
	_, err = fw.Write([]byte("audio"))
	require.NoError(t, err, "write audio bytes")
	require.NoError(t, writer.WriteField("model", "whisper-1"), "write model")
	require.NoError(t, writer.WriteField("language", strings.Repeat("x", 4096)), "write oversized language")
	for i := 0; i < 10; i++ {
		require.NoError(t, writer.WriteField("include[]", "logprobs"), "write repeated include")
	}
	for i := 0; i < 50; i++ {
		require.NoError(t, writer.WriteField(fmt.Sprintf("junk_%02d", i), "v"), "write junk field")
	}
	require.NoError(t, writer.Close(), "close writer")

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c, observed := newObservedAudioContext(t, req)
	require.Equal(t, "whisper-1", extractAudioModelFromMultipart(c))

	logAudioRequestParameters(c, relaymode.AudioTranscription, "whisper-1", nil)

	fields := audioParametersEntry(t, observed).ContextMap()

	language, ok := fields["language"].([]any)
	require.True(t, ok, "language must be recorded as a value list")
	require.Len(t, language, 1)
	require.Len(t, []rune(language[0].(string)), audioLogMaxValueChars+len("..."),
		"an oversized protocol value must be truncated, not logged whole")

	include, ok := fields["include[]"].([]any)
	require.True(t, ok, "include[] must be recorded as a value list")
	require.Len(t, include, audioLogMaxValuesPerName, "repeated values must be capped")

	require.EqualValues(t, 50, fields["unrecognized_field_count"],
		"the true count stays visible even when the sample is truncated")
	unknown, ok := fields["unrecognized_fields"].([]any)
	require.True(t, ok, "unrecognized_fields must be recorded as a name list")
	require.Len(t, unknown, audioLogMaxExtraFields, "the unknown-name sample must be capped")
}
