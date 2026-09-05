package controller

import (
	"path"
	"slices"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/relaymode"
)

// audioProtocolFormFields lists the transcription/translation form fields whose VALUES
// are safe to log: each is an enumerated protocol switch chosen from a fixed vocabulary,
// never caller content.
//
// Both the bare and the `[]`-suffixed spellings are listed because clients send array
// fields either way (curl and the Python SDK use `timestamp_granularities[]`, some HTTP
// libraries drop the brackets).
var audioProtocolFormFields = []string{
	"response_format",
	"language",
	"temperature",
	"stream",
	"chunking_strategy",
	"timestamp_granularities",
	"timestamp_granularities[]",
	"include",
	"include[]",
}

// audioContentFormFields lists fields that may carry caller content -- a biasing prompt,
// speaker names, a keyword list. Only their presence and length are logged, never the value.
var audioContentFormFields = []string{
	"prompt",
	"keywords",
	"keywords[]",
	"known_speaker_names",
	"known_speaker_names[]",
	"known_speaker_references",
	"known_speaker_references[]",
}

// Bounds applied to the emitted line. The form is caller-controlled, so every
// dimension of the log record -- value length, values per field, and the number of
// unrecognised field names -- is capped rather than trusted.
const (
	audioLogMaxValueChars    = 64
	audioLogMaxValuesPerName = 4
	audioLogMaxExtraFields   = 20
)

// logAudioRequestParameters emits a DEBUG line describing the protocol parameters of an
// audio request.
//
// Why this exists: the generic request logger deliberately records shape only for these
// endpoints (common.LogClientRequestPayload logs `body_logging_suppressed`), because a
// multipart body is mostly raw audio. So when an upstream rejects a parameter -- OpenAI
// answering 400 `response_format 'vtt' is not compatible with model 'gpt-transcribe-api-ev3'`,
// for instance -- the gateway's own logs previously said nothing about what the caller
// actually sent, and the upstream error text was the only evidence available.
//
// Privacy: audio bytes, the biasing prompt, keyword and speaker lists, filenames and
// credentials are never logged. Unrecognised fields contribute their NAME only, which is
// what makes an unknown-parameter report actionable without leaking its value.
//
// Parameters:
//   - c: the request context; its multipart form must already be parsed for form fields
//     to be reported (RelayAudioHelper binds the model first, which parses it).
//   - relayMode: the audio relay mode being served.
//   - audioModel: the model name after mapping, as it will be billed and forwarded.
//   - ttsRequest: the decoded speech request; only meaningful for relaymode.AudioSpeech.
func logAudioRequestParameters(c *gin.Context, relayMode int, audioModel string, ttsRequest *openai.TextToSpeechRequest) {
	lg := gmw.GetLogger(c)

	fields := []zap.Field{
		zap.String("audio_model", audioModel),
		zap.Int("relay_mode", relayMode),
	}

	if relayMode == relaymode.AudioSpeech {
		if ttsRequest != nil {
			fields = append(fields,
				zap.String("response_format", ttsRequest.ResponseFormat),
				zap.String("voice", ttsRequest.Voice),
				zap.Float64("speed", ttsRequest.Speed),
				zap.Int("input_chars", len(ttsRequest.Input)),
			)
		}
		lg.Debug("audio request parameters", fields...)
		return
	}

	form := c.Request.MultipartForm
	if form == nil {
		lg.Debug("audio request parameters", append(fields,
			zap.String("form_state", "not parsed"))...)
		return
	}

	var extraFields []string
	for name, values := range form.Value {
		switch {
		case name == "model":
			// Already reported as audio_model, after mapping.
		case slices.Contains(audioProtocolFormFields, name):
			fields = append(fields, zap.Strings(name, boundedValues(values)))
		case slices.Contains(audioContentFormFields, name):
			fields = append(fields,
				zap.Int(name+"_count", len(values)),
				zap.Int(name+"_chars", totalRuneLen(values)))
		default:
			extraFields = append(extraFields, name)
		}
	}
	if len(extraFields) > 0 {
		slices.Sort(extraFields)
		fields = append(fields, zap.Int("unrecognized_field_count", len(extraFields)))
		if len(extraFields) > audioLogMaxExtraFields {
			extraFields = extraFields[:audioLogMaxExtraFields]
		}
		fields = append(fields, zap.Strings("unrecognized_fields", extraFields))
	}

	for name, headers := range form.File {
		for _, header := range headers {
			// The filename itself is caller data; only its extension and size are
			// diagnostically useful.
			fields = append(fields,
				zap.String(name+"_ext", strings.ToLower(path.Ext(header.Filename))),
				zap.Int64(name+"_bytes", header.Size))
		}
	}

	lg.Debug("audio request parameters", fields...)
}

// boundedValues caps how much of a caller-supplied protocol field reaches the log: at
// most audioLogMaxValuesPerName entries, each truncated to audioLogMaxValueChars runes.
// Protocol switches are short by definition, so a longer value is itself the diagnosis.
func boundedValues(values []string) []string {
	if len(values) > audioLogMaxValuesPerName {
		values = values[:audioLogMaxValuesPerName]
	}
	bounded := make([]string, 0, len(values))
	for _, value := range values {
		runes := []rune(value)
		if len(runes) > audioLogMaxValueChars {
			bounded = append(bounded, string(runes[:audioLogMaxValueChars])+"...")
			continue
		}
		bounded = append(bounded, value)
	}
	return bounded
}

// totalRuneLen returns the combined character length of values, used to report the size
// of a content-bearing form field without logging the content itself.
func totalRuneLen(values []string) int {
	total := 0
	for _, value := range values {
		total += len([]rune(value))
	}
	return total
}
