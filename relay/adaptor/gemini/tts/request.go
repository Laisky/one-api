// Package tts converts standard speech requests and Gemini GenerateContent audio receipts.
package tts

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

const (
	MaxInputTokens  = 8192
	MaxOutputTokens = 16384
	MaxAudioBytes   = 32 << 20
	maxWireBytes    = 64 << 20
	sampleRate      = 24000
)

// Plan holds one validated request. It never stores credentials in diagnostic fields.
type Plan struct {
	Body        []byte
	Format      string
	Stream      bool
	SSE         bool
	Speed       float64
	OutputLimit int
	Encoder     string
}

// Turn is a verbatim transcript segment with separate delivery metadata.
type Turn struct {
	Text    string `json:"text"`
	Speaker string `json:"speaker"`
	Style   string `json:"style,omitempty"`
}

// Speaker assigns a prebuilt Gemini voice to a named dialogue participant.
type Speaker struct {
	Speaker string `json:"speaker"`
	Voice   string `json:"voice"`
}

// extension exposes bounded multi-speaker turns without permitting arbitrary native payload overrides.
type extension struct {
	Speakers        []Speaker `json:"speakers,omitempty"`
	Turns           []Turn    `json:"turns,omitempty"`
	MaxOutputTokens int       `json:"max_output_tokens,omitempty"`
}

// speechRequest is the accepted standard speech surface after extra_body normalization.
type speechRequest struct {
	Model          string    `json:"model"`
	Input          string    `json:"input"`
	Voice          string    `json:"voice"`
	Instructions   string    `json:"instructions,omitempty"`
	ResponseFormat string    `json:"response_format,omitempty"`
	StreamFormat   string    `json:"stream_format,omitempty"`
	Stream         bool      `json:"stream,omitempty"`
	Speed          *float64  `json:"speed,omitempty"`
	Gemini         extension `json:"gemini,omitempty"`
}

// CanonicalModelID returns a constant upstream identifier for a supported client model.
// Untrusted model text is never returned for interpolation into a network request URL.
func CanonicalModelID(model string) string {
	switch model {
	case "gemini-3.8-flash-tts":
		return "gemini-3.8-flash-tts"
	case "gemini-3.8-flash-lite-tts":
		return "gemini-3.8-flash-lite-tts"
	default:
		return ""
	}
}

// SupportsModel reports whether model uses the explicitly implemented 3.8 TTS protocol.
func SupportsModel(model string) bool {
	return CanonicalModelID(model) != ""
}

// strictJSON decodes exactly one JSON value, rejecting unknown fields and trailing data.
func strictJSON(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return errors.New("invalid speech JSON or unsupported field")
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("speech request contains trailing JSON")
	}
	return nil
}

// Prepare validates normalized client bytes and builds a native payload for actualModel.
// Unknown options, unmetered transcripts, and unavailable codecs fail before dispatch.
func Prepare(body []byte, actualModel string) (*Plan, error) {
	if !SupportsModel(actualModel) {
		return nil, errors.New("Gemini speech requires gemini-3.8-flash-tts or gemini-3.8-flash-lite-tts; legacy and Live models use different protocols")
	}
	if len(body) > 128<<10 || !utf8.Valid(body) {
		return nil, errors.New("speech request exceeds 128 KiB or is not valid UTF-8")
	}
	var req speechRequest
	if err := strictJSON(body, &req); err != nil {
		return nil, err
	}
	if req.Model != actualModel {
		return nil, errors.New("speech model mapping mismatch")
	}
	if strings.TrimSpace(req.Input) == "" || !utf8.ValidString(req.Input) || utf8.RuneCountInString(req.Input) > 4096 {
		return nil, errors.New("speech input must contain 1 to 4096 valid UTF-8 characters")
	}
	if utf8.RuneCountInString(req.Instructions) > 4096 {
		return nil, errors.New("speech instructions exceed 4096 characters")
	}
	plan := &Plan{Format: req.ResponseFormat, Stream: req.Stream, Speed: 1, OutputLimit: MaxOutputTokens}
	if plan.Format == "" {
		plan.Format = "mp3"
	}
	switch plan.Format {
	case "mp3", "opus", "aac", "flac", "wav", "pcm":
	default:
		return nil, errors.New("unsupported speech response_format")
	}
	switch req.StreamFormat {
	case "", "audio":
	case "sse":
		plan.SSE = true
		plan.Stream = true
	default:
		return nil, errors.New("speech stream_format must be audio or sse")
	}
	if req.Speed != nil {
		plan.Speed = *req.Speed
	}
	if math.IsNaN(plan.Speed) || math.IsInf(plan.Speed, 0) || plan.Speed < 0.25 || plan.Speed > 4 {
		return nil, errors.New("speech speed must be between 0.25 and 4")
	}
	if req.Gemini.MaxOutputTokens != 0 {
		plan.OutputLimit = req.Gemini.MaxOutputTokens
	}
	if plan.OutputLimit < 1 || plan.OutputLimit > MaxOutputTokens {
		return nil, errors.New("invalid Gemini speech output token limit")
	}
	parts, speechConfig, err := buildParts(req)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"contents": []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"maxOutputTokens":    plan.OutputLimit,
			"responseFormat":     map[string]any{"audio": map[string]any{"mimeType": "AUDIO_L16", "sampleRate": sampleRate}},
			"speechConfig":       speechConfig,
		},
	}
	plan.Body, err = json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "encode native speech request")
	}
	if plan.Speed != 1 || (plan.Format != "pcm" && plan.Format != "wav") {
		plan.Encoder, err = exec.LookPath("ffmpeg")
		if err != nil {
			return nil, errors.New("ffmpeg is required for this speech format or speed; use pcm or wav at speed 1")
		}
	}
	return plan, nil
}

// buildParts separates transcript and style, validating that dialogue cannot replace metered input.
func buildParts(req speechRequest) ([]any, map[string]any, error) {
	if len(req.Gemini.Speakers) == 0 && len(req.Gemini.Turns) == 0 {
		if strings.TrimSpace(req.Voice) == "" || len(req.Voice) > 8192 {
			return nil, nil, errors.New("a Gemini voice string is required")
		}
		part := map[string]any{"text": req.Input}
		if req.Instructions != "" {
			part["speech_metadata"] = map[string]string{"style": req.Instructions}
		}
		return []any{part}, map[string]any{"voiceConfig": map[string]string{"voice": req.Voice}}, nil
	}
	if req.Voice != "" || len(req.Gemini.Speakers) != 2 || len(req.Gemini.Turns) < 2 || len(req.Gemini.Turns) > 128 {
		return nil, nil, errors.New("dialogue requires exactly two speakers and 2 to 128 turns, without a top-level voice")
	}
	known := make(map[string]bool, 2)
	voices := make([]any, 0, 2)
	for _, speaker := range req.Gemini.Speakers {
		if strings.TrimSpace(speaker.Speaker) == "" || len(speaker.Speaker) > 128 || known[speaker.Speaker] || !prebuiltVoice(speaker.Voice) {
			return nil, nil, errors.New("dialogue speakers must have unique names and prebuilt Gemini voices")
		}
		known[speaker.Speaker] = true
		voices = append(voices, map[string]any{"speaker": speaker.Speaker, "voiceConfig": map[string]any{"prebuiltVoiceConfig": map[string]string{"voiceName": speaker.Voice}}})
	}
	parts := make([]any, 0, len(req.Gemini.Turns))
	var transcript strings.Builder
	styleChars := 0
	for _, turn := range req.Gemini.Turns {
		if !known[turn.Speaker] || turn.Text == "" {
			return nil, nil, errors.New("every dialogue turn needs text and a configured speaker")
		}
		transcript.WriteString(turn.Text)
		style := req.Instructions
		if turn.Style != "" {
			if style != "" {
				style += "\n"
			}
			style += turn.Style
		}
		styleChars += utf8.RuneCountInString(style)
		if styleChars > 4096 || transcript.Len() > len(req.Input) {
			return nil, nil, errors.New("dialogue transcript or style exceeds its limit")
		}
		parts = append(parts, map[string]any{"text": turn.Text, "speech_metadata": map[string]string{"speaker": turn.Speaker, "style": style}})
	}
	if transcript.String() != req.Input {
		return nil, nil, errors.New("dialogue turn text concatenation must exactly equal speech input")
	}
	return parts, map[string]any{"multiSpeakerVoiceConfig": map[string]any{"speakerVoiceConfigs": voices}}, nil
}

// prebuiltVoice restricts multi-speaker synthesis to Google's documented prebuilt voice set.
func prebuiltVoice(voice string) bool {
	switch voice {
	case "Zephyr", "Puck", "Charon", "Kore", "Fenrir", "Leda", "Orus", "Aoede", "Callirrhoe", "Autonoe", "Enceladus", "Iapetus", "Umbriel", "Algieba", "Despina", "Erinome", "Algenib", "Rasalgethi", "Laomedeia", "Achernar", "Alnilam", "Schedar", "Gacrux", "Pulcherrima", "Achird", "Zubenelgenubi", "Vindemiatrix", "Sadachbia", "Sadaltager", "Sulafat":
		return true
	default:
		return false
	}
}
