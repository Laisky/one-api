package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// nativeAudio builds deterministic provider receipts without accessing a paid provider.
func nativeAudio(t *testing.T, data []byte, mime, finish string, usage any) string {
	t.Helper()
	parts := []any{}
	if data != nil {
		parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": base64.StdEncoding.EncodeToString(data)}})
	}
	payload := map[string]any{"candidates": []any{map[string]any{"index": 0, "content": map[string]any{"parts": parts}, "finishReason": finish}}}
	if usage != nil {
		payload["usageMetadata"] = usage
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(raw)
}

// speechResponse creates a native HTTP response whose body belongs to the caller.
func speechResponse(body, contentType string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}, "X-Goog-Secret": []string{"never-copy"}}, Body: io.NopCloser(strings.NewReader(body))}
}

// completeUsage returns a small cumulative usage fixture with a cache hit.
func completeUsage() map[string]int {
	return map[string]int{"promptTokenCount": 12, "candidatesTokenCount": 2, "cachedContentTokenCount": 4}
}

// TestUnaryAudioAndWAV accepts both native encodings and writes exactly one WAV header.
func TestUnaryAudioAndWAV(t *testing.T) {
	t.Parallel()
	pcm := bytes.Repeat([]byte{1, 2}, 1920)
	for _, upstreamWAV := range []bool{false, true} {
		data, mime := pcm, "audio/L16;rate=24000;channels=1"
		if upstreamWAV {
			data, mime = makeWAV(pcm), "audio/wav"
		}
		for _, format := range []string{"pcm", "wav"} {
			plan := testPlan(t, map[string]any{"response_format": format})
			response := speechResponse(nativeAudio(t, data, mime, "STOP", completeUsage()), "application/json")
			w := httptest.NewRecorder()
			observed := false
			receipt, err := plan.Forward(context.Background(), response, w, func(r Receipt) { observed = r.Accepted })
			require.NoError(t, err)
			require.True(t, observed)
			require.True(t, receipt.UsageComplete)
			require.Equal(t, 12, receipt.PromptTokens)
			require.Equal(t, 2, receipt.OutputTokens)
			require.Equal(t, 4, receipt.CachedTokens)
			require.Empty(t, w.Header().Get("X-Goog-Secret"))
			if format == "wav" {
				require.Equal(t, makeWAV(pcm), w.Body.Bytes())
			} else {
				require.Equal(t, pcm, w.Body.Bytes())
			}
		}
	}
}

// TestStreamingCumulativeUsage verifies real chunk delivery, late usage, and exactly one done event.
func TestStreamingCumulativeUsage(t *testing.T) {
	t.Parallel()
	pcm := bytes.Repeat([]byte{1, 0}, 960)
	for _, sse := range []bool{false, true} {
		plan := testPlan(t, map[string]any{"stream": true})
		plan.SSE = sse
		first := nativeAudio(t, pcm, "audio/L16;rate=24000", "", map[string]int{"promptTokenCount": 12, "candidatesTokenCount": 1})
		second := nativeAudio(t, pcm, "audio/L16;rate=24000", "STOP", nil)
		body := ": comment\r\ndata: " + first + "\r\n\r\ndata: " + second + "\n\ndata: {\n" +
			"data: \"usageMetadata\":{\"promptTokenCount\":12,\"candidatesTokenCount\":2,\"cachedContentTokenCount\":4}}\n\n"
		w := httptest.NewRecorder()
		callbacks := 0
		r, err := plan.Forward(context.Background(), speechResponse(body, "text/event-stream; charset=utf-8"), w, func(r Receipt) { callbacks++ })
		require.NoError(t, err)
		require.Equal(t, 2, callbacks)
		require.True(t, w.Flushed)
		require.True(t, r.UsageComplete)
		require.Equal(t, 2, r.OutputTokens, "cumulative snapshots must not be added")
		require.Equal(t, 12, r.PromptTokens)
		if sse {
			require.Equal(t, 2, strings.Count(w.Body.String(), "event: speech.audio.delta"))
			require.Equal(t, 1, strings.Count(w.Body.String(), "event: speech.audio.done"))
			require.NotContains(t, w.Body.String(), "[DONE]")
		} else {
			require.Equal(t, append(append([]byte{}, pcm...), pcm...), w.Body.Bytes())
		}
	}
}

// TestStreamingErrorsKeepAcceptedWork rejects false successes without erasing already received audio.
func TestStreamingErrorsKeepAcceptedWork(t *testing.T) {
	t.Parallel()
	pcm := bytes.Repeat([]byte{1, 0}, 960)
	for _, suffix := range []string{"", "data: {\"error\":{\"message\":\"secret-transcript\"}}\n\n", "data: [DONE]\n\n", "data: {\"usageMetadata\":{\"promptTokenCount\":-1}}\n\n"} {
		plan := testPlan(t, map[string]any{"stream_format": "sse"})
		body := "data: " + nativeAudio(t, pcm, "audio/L16;rate=24000", "", nil) + "\n\n" + suffix
		w := httptest.NewRecorder()
		receipt, err := plan.Forward(context.Background(), speechResponse(body, "text/event-stream"), w, nil)
		require.Error(t, err)
		require.True(t, receipt.Accepted)
		require.False(t, receipt.UsageComplete)
		require.True(t, receipt.EstimatedOutput)
		require.Equal(t, 1, receipt.OutputTokens)
		require.Zero(t, receipt.PromptTokens, "missing input must not become a fabricated exact count")
		require.NotContains(t, w.Body.String(), "speech.audio.done")
		require.Contains(t, w.Body.String(), "event: error")
		require.NotContains(t, w.Body.String(), "secret-transcript")
		require.NotContains(t, err.Error(), "secret-transcript")
	}
}

// TestBadAudioReceipts validates malformed data, containers, MIME types and output budgets.
func TestBadAudioReceipts(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`null`, `{}`, `{"error":{"message":"private-voicekey"}}`,
		`{"candidates":[{"content":{"parts":[{"text":"not speech"}]},"finishReason":"STOP"}]}`,
		nativeAudio(t, []byte{1}, "audio/L16", "STOP", completeUsage()),
		nativeAudio(t, []byte{1, 2}, "audio/L16;rate=8000", "STOP", completeUsage()),
		nativeAudio(t, []byte{1, 2}, "audio/mpeg", "STOP", completeUsage()),
		nativeAudio(t, []byte{1, 2}, "audio/wav", "STOP", completeUsage()),
		strings.Replace(nativeAudio(t, []byte{1, 2}, "audio/L16", "STOP", completeUsage()), "AQI=", "!!!", 1),
	} {
		plan := testPlan(t, nil)
		r, err := plan.Forward(context.Background(), speechResponse(body, "application/json"), httptest.NewRecorder(), nil)
		require.Error(t, err)
		require.False(t, r.Accepted)
	}
	p := testPlan(t, map[string]any{"gemini": map[string]any{"max_output_tokens": 1}})
	r, err := p.Forward(context.Background(), speechResponse(nativeAudio(t, make([]byte, 1922), "audio/L16", "STOP", nil), "application/json"), httptest.NewRecorder(), nil)
	require.Error(t, err)
	require.False(t, r.Accepted)
}

// TestWAVChunkValidation checks additional RIFF chunks and rejects truncated or non-PCM audio.
func TestWAVChunkValidation(t *testing.T) {
	t.Parallel()
	pcm := bytes.Repeat([]byte{1, 2}, 100)
	wav := makeWAV(pcm)
	extra := append(append(append([]byte{}, wav[:36]...), []byte("JUNK\x02\x00\x00\x00ok")...), wav[36:]...)
	binary.LittleEndian.PutUint32(extra[4:], uint32(len(extra)-8))
	decoded, err := decodeWAV(extra)
	require.NoError(t, err)
	require.Equal(t, pcm, decoded)
	for _, mutate := range []func([]byte){
		func(b []byte) { b[20] = 3 }, func(b []byte) { b[22] = 2 }, func(b []byte) { b[24] = 1 },
		func(b []byte) { b[34] = 8 }, func(b []byte) { b[40] = 255 }, func(b []byte) { b[4] = 0 },
	} {
		invalid := append([]byte{}, wav...)
		mutate(invalid)
		_, err := decodeWAV(invalid)
		require.Error(t, err)
	}
}

// shortSpeechWriter simulates a disconnect without reporting a misleading successful write.
type shortSpeechWriter struct{ *httptest.ResponseRecorder }

// Write returns a short count to exercise downstream delivery error handling.
func (w shortSpeechWriter) Write(data []byte) (int, error) { return 0, nil }

// TestDownstreamFailureRetainsReceipt prevents a client disconnect from cancelling accepted billing.
func TestDownstreamFailureRetainsReceipt(t *testing.T) {
	t.Parallel()
	plan := testPlan(t, nil)
	response := speechResponse(nativeAudio(t, make([]byte, 3840), "audio/L16", "STOP", completeUsage()), "application/json")
	receipt, err := plan.Forward(context.Background(), response, shortSpeechWriter{httptest.NewRecorder()}, nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, io.ErrShortWrite))
	require.True(t, receipt.Accepted)
	require.True(t, receipt.UsageComplete)
	require.Equal(t, 2, receipt.OutputTokens)
}

// TestSpeechEncoderFormats performs real codec conversions using the runtime ffmpeg dependency.
func TestSpeechEncoderFormats(t *testing.T) {
	encoder, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for codec integration tests")
	}
	for _, format := range []string{"mp3", "aac", "flac", "opus", "pcm", "wav"} {
		for _, speed := range []float64{0.25, 1, 4} {
			p := &Plan{Encoder: encoder, Speed: speed, Format: format}
			encoded, err := p.encode(context.Background(), make([]byte, 48000))
			require.NoError(t, err, "%s speed %v", format, speed)
			require.NotEmpty(t, encoded)
			if format == "wav" {
				_, err = decodeWAV(encoded)
				require.NoError(t, err)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (&Plan{Encoder: encoder, Speed: 2, Format: "mp3"}).encode(ctx, make([]byte, 48000))
	require.Error(t, err)
}
