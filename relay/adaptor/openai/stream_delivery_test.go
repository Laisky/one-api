package openai

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// gatedStreamTail announces an upstream read, then withholds the remaining bytes until released.
// The channel handshake tests causality rather than relying on sleep-based latency assertions.
type gatedStreamTail struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	tail    *strings.Reader
}

// Read blocks before returning tail bytes and reports when the handler actually needs more upstream data.
func (g *gatedStreamTail) Read(p []byte) (int, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.tail.Read(p)
}

// TestStreamHandlerDeliversBeforeUpstreamContinues requires real HTTP delivery while the upstream is paused.
// Both first and subsequent content must arrive before another upstream write, including after ignored or partial lines.
func TestStreamHandlerDeliversBeforeUpstreamContinues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, buffered, tail string
	}{
		{"empty buffer", "", "data: [DONE]\n\n"},
		{"blank line", "\n", "data: [DONE]\n\n"},
		{"comments", ": keepalive\n\n: ignored\n", "data: [DONE]\n\n"},
		{"CRLF comments", ": keepalive\r\n\r\n", "data: [DONE]\r\n\r\n"},
		{"partial data line", "data: ", "[DONE]\n\n"},
		{"partial comment", ": partial", " comment\n\ndata: [DONE]\n\n"},
		{"ignored fields", "event: message\nid: 3\nretry: 1000\n", "data: [DONE]\n\n"},
		{"blank then partial", "\n: comment\n\ndata: ", "[DONE]\n\n"},
	} {
		for _, count := range []int{1, 2} {
			name := "first content/"
			if count == 2 {
				name = "subsequent content/"
			}
			t.Run(name+tc.name, func(t *testing.T) {
				wire := chatChunk("first", nil) + "\n\n"
				expected := "first"
				lastContent := `"content":"first"`
				if count == 2 {
					wire += chatChunk("second", nil) + "\n\n"
					expected += "second"
					lastContent = `"content":"second"`
				}
				gate := &gatedStreamTail{entered: make(chan struct{}), release: make(chan struct{}), tail: strings.NewReader(tc.tail)}
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(gate.release) }) }
				type outcome struct {
					err  *model.ErrorWithStatusCode
					text string
				}
				finished := make(chan outcome, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					c, _ := gin.CreateTestContext(w)
					c.Request = r
					upstream := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(io.MultiReader(strings.NewReader(wire+tc.buffered), gate))}
					err, text, _ := StreamHandler(c, upstream, relaymode.ChatCompletions)
					finished <- outcome{err: err, text: text}
				}))
				defer server.Close()
				// Release before server.Close even when an assertion fails, so the upstream reader cannot leak.
				defer release()
				client := server.Client()
				client.Timeout = 3 * time.Second
				response, err := client.Get(server.URL)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, http.StatusOK, response.StatusCode)
				select {
				case <-gate.entered:
				case <-time.After(3 * time.Second):
					t.Fatal("handler did not reach the gated upstream read")
				}
				scanner := bufio.NewScanner(response.Body)
				seen := false
				for scanner.Scan() {
					if strings.Contains(scanner.Text(), lastContent) {
						seen = true
						break
					}
				}
				require.NoError(t, scanner.Err(), "pending content must be flushed before reading more upstream bytes")
				require.True(t, seen, "content was withheld until the upstream continued")
				release()
				done := 0
				for scanner.Scan() {
					if scanner.Text() == "data: [DONE]" {
						done++
					}
				}
				require.NoError(t, scanner.Err())
				require.Equal(t, 1, done)
				select {
				case result := <-finished:
					require.Nil(t, result.err)
					require.Equal(t, expected, result.text)
				case <-time.After(3 * time.Second):
					t.Fatal("handler did not finish after the upstream was released")
				}
			})
		}
	}
}
