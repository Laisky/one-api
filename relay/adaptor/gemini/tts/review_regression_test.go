package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestSpeechMaxTokensDeliversBufferedAudio checks unary and streamed terminal truncation.
// Parameters: t is the test handle. Returns: none.
func TestSpeechMaxTokensDeliversBufferedAudio(t *testing.T) {
	t.Parallel()
	pcm := bytes.Repeat([]byte{1, 2}, 1920)
	for _, format := range []string{"pcm", "wav", "mp3"} {
		for _, mode := range []string{"unary", "binary_stream", "sse"} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				plan := &Plan{Format: format, Speed: 1, OutputLimit: 2, Stream: mode != "unary", SSE: mode == "sse"}
				if format == "mp3" {
					encoder, err := exec.LookPath("ffmpeg")
					if err != nil {
						t.Skip("ffmpeg is required for real MP3 integration")
					}
					plan.Encoder = encoder
				}
				body := nativeAudio(t, pcm, "audio/L16;rate=24000", "MAX_TOKENS", completeUsage())
				contentType := "application/json"
				if plan.Stream {
					contentType = "text/event-stream"
					first := nativeAudio(t, pcm[:1920], "audio/L16", "", map[string]int{"promptTokenCount": 12, "candidatesTokenCount": 1})
					last := nativeAudio(t, pcm[1920:], "audio/L16", "MAX_TOKENS", nil)
					body = "data: " + first + "\n\ndata: " + last + "\n\n" +
						"data: {\"usageMetadata\":{\"promptTokenCount\":12,\"candidatesTokenCount\":2,\"cachedContentTokenCount\":4}}\n\n"
				}
				w := httptest.NewRecorder()
				accepted := false
				receipt, err := plan.Forward(context.Background(), speechResponse(body, contentType), w, func(r Receipt) { accepted = r.Accepted })
				require.NoError(t, err)
				require.True(t, accepted)
				require.True(t, receipt.Accepted)
				require.True(t, receipt.Truncated)
				require.True(t, receipt.UsageComplete, "late cumulative usage must still be read")
				require.False(t, receipt.EncodingFailed)
				require.False(t, receipt.EstimatedOutput)
				require.Equal(t, 12, receipt.PromptTokens)
				require.Equal(t, 2, receipt.OutputTokens)
				require.Equal(t, 4, receipt.CachedTokens)
				require.Equal(t, len(pcm), receipt.AudioBytes)
				audio := w.Body.Bytes()
				if plan.SSE {
					audio = nil
					done := 0
					for _, line := range strings.Split(w.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: ") {
							continue
						}
						var event map[string]json.RawMessage
						require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
						var kind string
						require.NoError(t, json.Unmarshal(event["type"], &kind))
						switch kind {
						case "speech.audio.delta":
							var encoded string
							require.NoError(t, json.Unmarshal(event["audio"], &encoded))
							chunk, err := base64.StdEncoding.DecodeString(encoded)
							require.NoError(t, err)
							audio = append(audio, chunk...)
						case "speech.audio.done":
							done++
							require.JSONEq(t, `true`, string(event["truncated"]))
						default:
							t.Fatalf("unexpected speech event %q", kind)
						}
					}
					require.Equal(t, 1, done)
				}
				switch format {
				case "pcm":
					require.Equal(t, pcm, audio)
				case "wav":
					require.Equal(t, makeWAV(pcm), audio)
				case "mp3":
					require.NotEmpty(t, audio)
					require.NotEqual(t, pcm, audio)
				}
				if !plan.Stream || format != "pcm" {
					require.Equal(t, "MAX_TOKENS", w.Result().Header.Get("X-Gemini-Finish-Reason"))
				}
			})
		}
	}
}

// TestSpeechMaxTokensDoesNotBypassValidation keeps refusal, budget, and late-data guards.
// Parameters: t is the test handle. Returns: none.
func TestSpeechMaxTokensDoesNotBypassValidation(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		nativeAudio(t, nil, "audio/L16", "MAX_TOKENS", completeUsage()),
		nativeAudio(t, []byte{1, 2}, "audio/L16", "SAFETY", completeUsage()),
		nativeAudio(t, make([]byte, 3842), "audio/L16", "MAX_TOKENS", completeUsage()),
	} {
		plan := &Plan{Format: "wav", Speed: 1, OutputLimit: 2}
		w := httptest.NewRecorder()
		receipt, err := plan.Forward(context.Background(), speechResponse(body, "application/json"), w, nil)
		require.Error(t, err)
		require.False(t, receipt.Accepted)
		require.Empty(t, w.Body.Bytes())
	}
	for _, suffix := range []string{
		"data: " + nativeAudio(t, []byte{1, 2}, "audio/L16", "", nil) + "\n\n",
		"data: {\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}\n\n",
	} {
		plan := &Plan{Format: "pcm", Speed: 1, OutputLimit: 2, Stream: true, SSE: true}
		body := "data: " + nativeAudio(t, make([]byte, 3840), "audio/L16", "MAX_TOKENS", completeUsage()) + "\n\n" + suffix
		w := httptest.NewRecorder()
		receipt, err := plan.Forward(context.Background(), speechResponse(body, "text/event-stream"), w, nil)
		require.Error(t, err)
		require.True(t, receipt.Accepted)
		require.False(t, receipt.UsageComplete)
		require.NotContains(t, w.Body.String(), "speech.audio.done")
	}
}

// TestSpeechEncodingFailureKeepsReceiptForCredit separates internal conversion from provider acceptance.
// Parameters: t is the test handle. Returns: none.
func TestSpeechEncodingFailureKeepsReceiptForCredit(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		plan := &Plan{Format: "mp3", Speed: 1, OutputLimit: 2, Stream: stream, SSE: stream, Encoder: filepath.Join(t.TempDir(), "missing-encoder")}
		body := nativeAudio(t, make([]byte, 3840), "audio/L16", "STOP", completeUsage())
		contentType := "application/json"
		if stream {
			body, contentType = "data: "+body+"\n\n", "text/event-stream"
		}
		w := httptest.NewRecorder()
		accepted := false
		receipt, err := plan.Forward(context.Background(), speechResponse(body, contentType), w, func(r Receipt) { accepted = r.Accepted })
		require.Error(t, err)
		require.True(t, accepted, "provider acceptance must still prohibit replay")
		require.True(t, receipt.Accepted)
		require.True(t, receipt.EncodingFailed)
		require.True(t, receipt.UsageComplete)
		require.Equal(t, 2, receipt.OutputTokens)
		require.Empty(t, w.Body.Bytes(), "no audio or success event may precede a failed buffered conversion")
		require.Empty(t, w.Header().Get("Content-Type"), "failure must leave the response uncommitted")
		require.NotContains(t, err.Error(), plan.Encoder)
	}
}

// cancelAtSpeechEOF cancels after native events have been consumed but before buffered conversion.
type cancelAtSpeechEOF struct {
	io.Reader
	cancel context.CancelFunc
}

// Read forwards bytes and cancels its context upon reaching EOF.
// Parameters: p is the destination. Returns: the underlying count and error.
func (r cancelAtSpeechEOF) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

// TestSpeechCancellationAndWriteFailureDoNotRequestCredit retains accepted-work charging.
// Parameters: t is the test handle. Returns: none.
func TestSpeechCancellationAndWriteFailureDoNotRequestCredit(t *testing.T) {
	t.Parallel()
	body := nativeAudio(t, make([]byte, 3840), "audio/L16", "STOP", completeUsage())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plan := &Plan{Format: "mp3", Speed: 1, OutputLimit: 2, Stream: true, Encoder: filepath.Join(t.TempDir(), "missing-encoder")}
	resp := speechResponse("", "text/event-stream")
	resp.Body = io.NopCloser(cancelAtSpeechEOF{Reader: strings.NewReader("data: " + body + "\n\n"), cancel: cancel})
	receipt, err := plan.Forward(ctx, resp, httptest.NewRecorder(), nil)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, receipt.Accepted)
	require.False(t, receipt.EncodingFailed)

	plan = &Plan{Format: "wav", Speed: 1, OutputLimit: 2}
	receipt, err = plan.Forward(context.Background(), speechResponse(body, "application/json"), shortSpeechWriter{httptest.NewRecorder()}, nil)
	require.True(t, errors.Is(err, io.ErrShortWrite))
	require.True(t, receipt.Accepted)
	require.False(t, receipt.EncodingFailed)
}
