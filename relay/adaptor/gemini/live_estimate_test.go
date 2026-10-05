package gemini

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/realtime"
)

// liveTestImage encodes a blank image. Parameters: t owns assertions, width and
// height size it and format selects jpeg or png. Returns: base64 image data.
func liveTestImage(t *testing.T, width, height int, format string) string {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	var encoded bytes.Buffer
	if format == "png" {
		require.NoError(t, png.Encode(&encoded, picture))
	} else {
		require.NoError(t, jpeg.Encode(&encoded, picture, nil))
	}
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

// TestEstimateLiveClientFrameBounds verifies conservative per-modality input
// bounds that do not depend on elapsed session time. Parameters: t owns the
// test. Returns: none.
func TestEstimateLiveClientFrameBounds(t *testing.T) {
	t.Parallel()
	second := base64.StdEncoding.EncodeToString(make([]byte, 32000)) // 1 s at 16 kHz
	wide := liveTestImage(t, 3000, 3000, "jpeg")
	small := liveTestImage(t, 10, 10, "png")
	for _, tc := range []struct {
		name  string
		frame string
		check func(t *testing.T, e realtime.Estimate)
	}{
		{"utf8_text_bytes", `{"realtimeInput":{"text":"héllo"}}`, func(t *testing.T, e realtime.Estimate) {
			require.Equal(t, realtime.Estimate{Text: 6}, e)
		}},
		{"pcm_default_rate", `{"realtimeInput":{"audio":{"mimeType":"audio/pcm","data":"` + second + `"}}}`, func(t *testing.T, e realtime.Estimate) {
			require.GreaterOrEqual(t, e.Audio, int64(liveAudioTokensPerSecond))
			require.LessOrEqual(t, e.Audio, int64(liveAudioTokensPerSecond+1))
		}},
		{"pcm_declared_rate", `{"realtimeInput":{"audio":{"mimeType":"audio/PCM; rate=8000","data":"` + second + `"}}}`, func(t *testing.T, e realtime.Estimate) {
			require.GreaterOrEqual(t, e.Audio, int64(2*liveAudioTokensPerSecond), "a lower sample rate is a longer duration")
		}},
		{"visual_tiles", `{"realtimeInput":{"video":{"mimeType":"image/jpeg","data":"` + wide + `"}}}`, func(t *testing.T, e realtime.Estimate) {
			require.Equal(t, realtime.Estimate{Image: 16 * liveImageTileTokens}, e)
		}},
		{"visual_floor", `{"realtimeInput":{"mediaChunks":[{"mime_type":"image/png","data":"` + small + `"},{"mimeType":"audio/pcm;rate=16000","data":"` + second + `"}]}}`, func(t *testing.T, e realtime.Estimate) {
			require.EqualValues(t, liveImageTokenFloor, e.Image)
			require.Positive(t, e.Audio)
		}},
		{"controls_carry_no_input", `{"realtimeInput":{"activityStart":{}}}`, func(t *testing.T, e realtime.Estimate) {
			require.True(t, e.IsZero())
		}},
		{"content_inline_media_excluded_from_text", `{"clientContent":{"turns":[{"role":"user","parts":[{"text":"hi"},{"inline_data":{"mime_type":"image/png","data":"` + small + `"}}]}],"turnComplete":true}}`, func(t *testing.T, e realtime.Estimate) {
			require.EqualValues(t, liveImageTokenFloor, e.Image)
			require.Less(t, e.Text, int64(200), "base64 media is priced by modality, not as text")
			require.Positive(t, e.Text)
		}},
		{"function_result_bytes", `{"toolResponse":{"functionResponses":[{"id":"a","name":"f","response":{"v":"` + strings.Repeat("z", 500) + `"}}]}}`, func(t *testing.T, e realtime.Estimate) {
			require.Greater(t, e.Text, int64(500))
		}},
		{"inline_text_document", `{"clientContent":{"turns":[{"parts":[{"inlineData":{"mimeType":"text/plain","data":"` + base64.StdEncoding.EncodeToString([]byte("abc")) + `"}}]}]}}`, func(t *testing.T, e realtime.Estimate) {
			require.GreaterOrEqual(t, e.Text, int64(3))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			estimate, err := estimateLiveClientFrame([]byte(tc.frame))
			require.NoError(t, err)
			tc.check(t, estimate)
		})
	}
}

// TestEstimateLiveClientFrameFailsClosed verifies that work whose cost cannot be
// bounded locally is refused instead of forwarded. Parameters: t owns the test.
func TestEstimateLiveClientFrameFailsClosed(t *testing.T) {
	t.Parallel()
	for name, frame := range map[string]string{
		"file_reference":    `{"clientContent":{"turns":[{"parts":[{"fileData":{"fileUri":"gs://b/o","mimeType":"video/mp4"}}]}],"turnComplete":true}}`,
		"snake_file_ref":    `{"toolResponse":{"functionResponses":[{"id":"a","name":"f","response":{"parts":[{"file_data":{"file_uri":"gs://b/o"}}]}}]}}`,
		"compressed_audio":  `{"realtimeInput":{"audio":{"mimeType":"audio/ogg","data":"AAAA"}}}`,
		"invalid_rate":      `{"realtimeInput":{"audio":{"mimeType":"audio/pcm;rate=abc","data":"AAAA"}}}`,
		"zero_rate":         `{"realtimeInput":{"audio":{"mimeType":"audio/pcm;rate=0","data":"AAAA"}}}`,
		"undecodable_image": `{"realtimeInput":{"video":{"mimeType":"image/jpeg","data":"AAAA"}}}`,
		"inline_video":      `{"clientContent":{"turns":[{"parts":[{"inlineData":{"mimeType":"video/mp4","data":"AAAA"}}]}]}}`,
		"missing_mime":      `{"realtimeInput":{"video":{"data":"AAAA"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := estimateLiveClientFrame([]byte(frame))
			require.ErrorIs(t, err, realtime.ErrUnpriceableInput)
			require.Equal(t, liveCloseUnpriceableWork, liveSpendCloseReason(err))
		})
	}
}

// TestEstimateLiveServerOutput verifies streamed output bounds for audio, text,
// transcription and function calls. Parameters: t owns the test. Returns: none.
func TestEstimateLiveServerOutput(t *testing.T) {
	t.Parallel()
	second := base64.StdEncoding.EncodeToString(make([]byte, 48000)) // 1 s at 24 kHz
	audio := estimateLiveServerOutput([]byte(`{"serverContent":{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=24000","data":"` + second + `"}},{"text":"abc"}]},"outputTranscription":{"text":"hello"}}}`))
	require.GreaterOrEqual(t, audio.OutputAudio, int64(liveAudioTokensPerSecond))
	require.LessOrEqual(t, audio.OutputAudio, int64(liveAudioTokensPerSecond+1))
	require.EqualValues(t, 8, audio.OutputText)
	call := estimateLiveServerOutput([]byte(`{"toolCall":{"functionCalls":[{"id":"1","name":"f","args":{}}]}}`))
	require.Positive(t, call.OutputText)
	require.True(t, estimateLiveServerOutput([]byte(`{"setupComplete":{}}`)).IsZero())
}

// TestLiveTurnOutputAllowance verifies the provider-enforced output cap is used
// when the client sets one. Parameters: t owns the test. Returns: none.
func TestLiveTurnOutputAllowance(t *testing.T) {
	t.Parallel()
	require.EqualValues(t, liveDefaultTurnOutputTokens, liveTurnOutputAllowance([]byte(`{"setup":{}}`)))
	require.EqualValues(t, 500, liveTurnOutputAllowance([]byte(`{"setup":{"generationConfig":{"maxOutputTokens":500}}}`)))
	require.EqualValues(t, 90000, liveTurnOutputAllowance([]byte(`{"setup":{"generationConfig":{"maxOutputTokens":90000}}}`)))
	require.EqualValues(t, liveDefaultTurnOutputTokens, liveTurnOutputAllowance([]byte(`{"setup":{"generationConfig":{"maxOutputTokens":0}}}`)))
	require.EqualValues(t, liveDefaultTurnOutputTokens, liveTurnOutputAllowance([]byte(`{"setup":{"generationConfig":{"maxOutputTokens":-4}}}`)))
	require.EqualValues(t, 100000, liveTurnOutputAllowance([]byte(`{"setup":{"generationConfig":{"maxOutputTokens":1e5}}}`)))
	require.EqualValues(t, 501, liveTurnOutputAllowance([]byte(`{"setup":{"generationConfig":{"maxOutputTokens":500.5}}}`)))
}
