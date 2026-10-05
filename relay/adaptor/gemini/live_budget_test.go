package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestGeminiLiveAggregateInputBoundary reproduces aggregate input admission on
// real provider/client sockets. Parameters: t owns the test. Returns: none; the
// provider must receive legitimate work, but not the frame crossing eight MiB.
// This resource boundary alone does not establish a prepaid monetary ceiling.
func TestGeminiLiveAggregateInputBoundary(t *testing.T) {
	const frameBytes = 1 << 20
	for _, operation := range []string{"text", "audio", "video"} {
		t.Run(operation, func(t *testing.T) {
			frame := liveBudgetFixtureFrame(t, operation, frameBytes)
			observed := make(chan int, 1)
			endpoint, results := liveFixture(t, channeltype.Gemini, "gemini-3.8-live", func(up *websocket.Conn) error {
				if err := acknowledgeLiveFixture(up, "gemini-3.8-live"); err != nil {
					return errors.Wrap(err, "acknowledge aggregate fixture")
				}
				count := 0
				defer func() { observed <- count }()
				for count < 9 {
					_, raw, err := up.ReadMessage()
					if err != nil {
						return nil // Session teardown is the boundary under test.
					}
					if string(raw) != frame {
						return errors.New("aggregate fixture frame changed")
					}
					count++
				}
				liveClose(up, websocket.CloseNormalClosure, "fixture_complete")
				return nil
			})
			client := connectLiveFixture(t, endpoint)
			for range 9 {
				if err := client.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
					break // A bounded gateway may close before the final write.
				}
			}
			usage := receiveLiveFixture(t, results)
			select {
			case count := <-observed:
				// Setup consumes part of the same eight-MiB budget.
				require.Equal(t, 7, count, "provider received work beyond the aggregate input allowance")
			case <-time.After(time.Second):
				t.Fatal("provider result missing")
			}
			require.True(t, usage.Realtime.HasUsageGap(), "missing receipts must remain unresolved")
		})
	}
}

// liveBudgetFixtureFrame returns an exact-size native input frame for modality.
// Parameters: t owns assertions, modality selects input and size bounds its JSON.
// Returns: valid JSON with text or base64 data; no real provider receives it.
func liveBudgetFixtureFrame(t *testing.T, modality string, size int) string {
	t.Helper()
	prefix, suffix := `{"realtimeInput":{"text":"`, `"}}`
	switch modality {
	case "audio":
		prefix, suffix = `{"realtimeInput":{"audio":{"mimeType":"audio/pcm;rate=16000","data":"`, `"}}}`
	case "video":
		prefix, suffix = `{"realtimeInput":{"video":{"mimeType":"image/jpeg","data":"`, `"}}}`
	}
	data := strings.Repeat("A", size-len(prefix)-len(suffix))
	if modality == "video" {
		data = liveBudgetVisualData(t)
		require.LessOrEqual(t, len(prefix)+len(data)+len(suffix), size)
		suffix += strings.Repeat(" ", size-len(prefix)-len(data)-len(suffix))
	}
	if modality != "text" {
		// JSON trailing whitespace preserves valid base64 alignment and exact bytes.
		padding := len(data) % 4
		data = data[:len(data)-padding]
		suffix += strings.Repeat(" ", padding)
	}
	frame := prefix + data + suffix
	require.Len(t, frame, size)
	require.True(t, json.Valid([]byte(frame)))
	return frame
}

// liveBudgetVisualData constructs a genuine bounded JPEG for the loopback test.
// Parameters: t owns encoding assertions. Returns: base64 JPEG data; deterministic
// pixel noise prevents the image from collapsing into a tiny compressed payload.
func liveBudgetVisualData(t *testing.T) string {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 768, 768))
	var seed uint32 = 1
	for i := range picture.Pix {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		picture.Pix[i] = byte(seed)
		if i%4 == 3 {
			picture.Pix[i] = 255
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: 95}))
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}
