package render

import (
	"bytes"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
)

var errStringDataTestWrite = errors.New("injected SSE string write failure")

// stringDataObserver captures write boundaries and flushes, with optional deterministic downstream faults.
type stringDataObserver struct {
	gin.ResponseWriter
	segments []string
	flushes  int
	failAt   int
	short    bool
}

// Write records bytes and fails the selected segment, otherwise delegating to the underlying response writer.
func (w *stringDataObserver) Write(data []byte) (int, error) {
	w.segments = append(w.segments, string(data))
	if w.failAt == len(w.segments) {
		if w.short {
			return max(0, len(data)-1), nil
		}
		return 0, errStringDataTestWrite
	}
	return w.ResponseWriter.Write(data)
}

// WriteString uses the same observer so io.StringWriter and io.Writer paths remain comparable.
func (w *stringDataObserver) WriteString(data string) (int, error) { return w.Write([]byte(data)) }

// Flush counts and forwards each flush; the test requires every call to StringData to flush independently.
func (w *stringDataObserver) Flush() { w.flushes++; w.ResponseWriter.Flush() }

// legacyStringData is the unchanged pre-optimization wire contract, deliberately independent of StringData.
func legacyStringData(c *gin.Context, data string) {
	data = strings.TrimPrefix(data, "data: ")
	data = strings.TrimSuffix(data, "\r")
	c.Render(-1, common.CustomEvent{Data: "data: " + data})
	c.Writer.Flush()
}

// stringDataOutcome holds observable wire data, headers, error identity, abortion and flush boundaries.
type stringDataOutcome struct {
	body                   string
	header                 http.Header
	segments               []string
	flushes                int
	aborted                bool
	errors                 []string
	writeError, shortError bool
}

// observeStringData invokes render for a sequence and captures its client-visible and failure behavior.
func observeStringData(render func(*gin.Context, string), payloads []string, failAt int, short bool) stringDataOutcome {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Header("Cache-Control", "private, no-store")
	writer := &stringDataObserver{ResponseWriter: c.Writer, failAt: failAt, short: short}
	c.Writer = writer
	for _, payload := range payloads {
		render(c, payload)
	}
	out := stringDataOutcome{body: recorder.Body.String(), header: recorder.Header().Clone(), segments: writer.segments, flushes: writer.flushes, aborted: c.IsAborted()}
	for _, err := range c.Errors {
		out.errors = append(out.errors, err.Error())
		out.writeError = out.writeError || errors.Is(err.Err, errStringDataTestWrite)
		out.shortError = out.shortError || errors.Is(err.Err, io.ErrShortWrite)
	}
	return out
}

// TestStringDataWireEquivalence preserves framing, arbitrary bytes, headers and one flush per submitted event.
func TestStringDataWireEquivalence(t *testing.T) {
	payloads := []string{"", "data:", "data: ", "data:  x", "data:x", "[DONE]", "data: [DONE]", "\r", "data: \r\r", "x\ny\rz\r", "世界🙂", "data: \x00\xff\xc0\x80", "event: value", "data: " + strings.Repeat("x", 128<<10)}
	rng := rand.New(rand.NewPCG(427, 20260925))
	for i := 0; i < 512; i++ {
		data := make([]byte, rng.IntN(512))
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		payloads = append(payloads, string(data), "data: "+string(data))
	}
	want := observeStringData(legacyStringData, payloads, 0, false)
	got := observeStringData(StringData, payloads, 0, false)
	require.Equal(t, want, got)
	require.Equal(t, len(payloads), got.flushes, "do not batch or defer SSE delivery")
	require.Equal(t, "text/event-stream", got.header.Get("Content-Type"))
	require.Equal(t, "private, no-store", got.header.Get("Cache-Control"))
}

// TestStringDataFailureEquivalence preserves abort, error identity and partial-write boundaries for every segment.
func TestStringDataFailureEquivalence(t *testing.T) {
	for _, short := range []bool{false, true} {
		for _, failAt := range []int{1, 2} {
			want := observeStringData(legacyStringData, []string{"data: hello\nworld\r"}, failAt, short)
			got := observeStringData(StringData, []string{"data: hello\nworld\r"}, failAt, short)
			require.Equal(t, want, got)
			require.True(t, got.aborted)
			require.Len(t, got.errors, 1)
			require.Equal(t, short, got.shortError)
			require.Equal(t, !short, got.writeError)
		}
	}
}

// stringDataDiscard is a fixed-memory HTTP writer for allocation benchmarks, not a buffering client substitute.
type stringDataDiscard struct{ header http.Header }

// Header returns the benchmark's reusable header map.
func (w *stringDataDiscard) Header() http.Header { return w.header }

// Write reports a successful byte write without retaining the payload.
func (w *stringDataDiscard) Write(p []byte) (int, error) { return len(p), nil }

// WriteString reports a successful string write without allocating a byte conversion.
func (w *stringDataDiscard) WriteString(p string) (int, error) { return len(p), nil }

// WriteHeader preserves the http.ResponseWriter contract without recording timings.
func (w *stringDataDiscard) WriteHeader(int) {}

// Flush keeps the same invocation path while avoiding network overhead in the microbenchmark.
func (w *stringDataDiscard) Flush() {}

// BenchmarkStringDataCopies compares the retained generic renderer with the current implementation using identical payloads.
func BenchmarkStringDataCopies(b *testing.B) {
	for name, render := range map[string]func(*gin.Context, string){"legacy": legacyStringData, "current": StringData} {
		b.Run(name, func(b *testing.B) {
			c, _ := gin.CreateTestContext(&stringDataDiscard{header: make(http.Header)})
			payload := "data: " + string(bytes.Repeat([]byte("x"), 512))
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				render(c, payload)
			}
		})
	}
}
