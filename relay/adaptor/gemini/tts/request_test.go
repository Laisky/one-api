package tts

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testPlan constructs a normalized request and exercises the public request converter.
func testPlan(t *testing.T, fields map[string]any) *Plan {
	t.Helper()
	request := map[string]any{"model": "gemini-3.8-flash-tts", "input": "Hello!", "voice": "Kore", "response_format": "pcm"}
	for key, value := range fields {
		request[key] = value
	}
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	plan, err := Prepare(raw, request["model"].(string))
	require.NoError(t, err)
	return plan
}

// TestPrepareVerbatimSpeech verifies both released models and separates style from spoken input.
func TestPrepareVerbatimSpeech(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
		t.Run(name, func(t *testing.T) {
			text := "  Hello <sigh>!\nKeep these words.  "
			plan := testPlan(t, map[string]any{"model": name, "input": text, "instructions": "calm and friendly", "voice": "voicekey_private-fixture"})
			var native map[string]any
			require.NoError(t, json.Unmarshal(plan.Body, &native))
			contents := native["contents"].([]any)
			require.Len(t, contents, 1)
			parts := contents[0].(map[string]any)["parts"].([]any)
			require.Equal(t, text, parts[0].(map[string]any)["text"])
			require.Equal(t, map[string]any{"style": "calm and friendly"}, parts[0].(map[string]any)["speech_metadata"])
			config := native["generationConfig"].(map[string]any)
			require.Equal(t, []any{"AUDIO"}, config["responseModalities"])
			require.Equal(t, map[string]any{"audio": map[string]any{"mimeType": "AUDIO_L16", "sampleRate": float64(24000)}}, config["responseFormat"])
			require.Equal(t, map[string]any{"voiceConfig": map[string]any{"voice": "voicekey_private-fixture"}}, config["speechConfig"])
			require.NotContains(t, native, "tools")
			require.NotContains(t, native, "systemInstruction")
		})
	}
}

// TestPrepareRejectsUnsupportedInput proves validation happens without any provider operation.
func TestPrepareRejectsUnsupportedInput(t *testing.T) {
	t.Parallel()
	for _, fields := range []map[string]any{
		{"input": ""}, {"input": strings.Repeat("é", 4097)}, {"voice": ""},
		{"instructions": strings.Repeat("x", 4097)}, {"speed": 0}, {"speed": 4.1},
		{"response_format": "ogg"}, {"stream_format": "realtime"}, {"seed": 1},
		{"gemini": map[string]any{"max_output_tokens": -1}},
		{"gemini": map[string]any{"max_output_tokens": 16385}},
		{"gemini": map[string]any{"contents": []any{}}},
	} {
		req := map[string]any{"model": "gemini-3.8-flash-tts", "input": "hello", "voice": "Kore", "response_format": "pcm"}
		for k, v := range fields {
			req[k] = v
		}
		raw, err := json.Marshal(req)
		require.NoError(t, err)
		_, err = Prepare(raw, "gemini-3.8-flash-tts")
		require.Error(t, err, "%v", fields)
	}
	for _, raw := range []string{`null`, `[]`, `{}`, `{"model":`, `{} {}`, `{"private-secret-field":"value"}`} {
		_, err := Prepare([]byte(raw), "gemini-3.8-flash-tts")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private-secret-field")
	}
	for _, name := range []string{"gemini-3.8-live", "gemini-2.5-pro-preview-tts", "gemini-3.1-flash-tts-preview", "gemini-99-tts"} {
		_, err := Prepare(nil, name)
		require.Error(t, err)
		require.False(t, SupportsModel(name))
	}
}

// TestPrepareDialogue validates two-speaker configuration without an alternate unmetered transcript.
func TestPrepareDialogue(t *testing.T) {
	t.Parallel()
	req := speechRequest{Model: "gemini-3.8-flash-tts", Input: "Hello!Hi!", ResponseFormat: "pcm", Gemini: extension{
		Speakers: []Speaker{{Speaker: "Joe", Voice: "Puck"}, {Speaker: "Jane", Voice: "Kore"}},
		Turns:    []Turn{{Speaker: "Joe", Text: "Hello!", Style: "friendly"}, {Speaker: "Jane", Text: "Hi!"}},
	}}
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	plan, err := Prepare(raw, req.Model)
	require.NoError(t, err)
	var native map[string]any
	require.NoError(t, json.Unmarshal(plan.Body, &native))
	parts := native["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 2)
	require.Equal(t, map[string]any{"speaker": "Joe", "style": "friendly"}, parts[0].(map[string]any)["speech_metadata"])
	for _, change := range []func(*speechRequest){
		func(r *speechRequest) { r.Input = "unrelated cheap input" },
		func(r *speechRequest) { r.Voice = "Kore" },
		func(r *speechRequest) { r.Gemini.Speakers[1].Speaker = "Joe" },
		func(r *speechRequest) { r.Gemini.Speakers[0].Voice = "voicekey_private" },
		func(r *speechRequest) { r.Gemini.Turns[0].Speaker = "unknown" },
	} {
		var invalid speechRequest
		require.NoError(t, json.Unmarshal(raw, &invalid))
		change(&invalid)
		input, err := json.Marshal(invalid)
		require.NoError(t, err)
		_, err = Prepare(input, invalid.Model)
		require.Error(t, err)
	}
}

// TestPrepareStreamingAndEncoderAvailability verifies native audio works without ffmpeg.
func TestPrepareStreamingAndEncoderAvailability(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, format := range []string{"wav", "pcm"} {
		p := testPlan(t, map[string]any{"response_format": format, "stream_format": "sse"})
		require.True(t, p.Stream)
		require.True(t, p.SSE)
		require.Empty(t, p.Encoder)
	}
	_, err := Prepare([]byte(`{"model":"gemini-3.8-flash-tts","input":"hello","voice":"Kore"}`), "gemini-3.8-flash-tts")
	require.ErrorContains(t, err, "ffmpeg")
}
