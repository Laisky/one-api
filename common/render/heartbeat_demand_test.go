package render

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/stretchr/testify/require"
)

// demandProbe records upstream reads so the test can distinguish demand from unsafe read-ahead.
type demandProbe struct {
	source *strings.Reader
	calls  atomic.Int64
}

// Read copies source bytes into p and records how many underlying reads occurred.
func (r *demandProbe) Read(p []byte) (int, error) {
	r.calls.Add(1)
	return r.source.Read(p)
}

// TestHeartbeatDemandPreservesOversizedOwnership verifies that a worker never reads ahead while its caller owns Line.Large.
func TestHeartbeatDemandPreservesOversizedOwnership(t *testing.T) {
	payload := strings.Repeat("hello 世界🙂", 12000)
	source := &demandProbe{source: strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n")}
	h := NewHeartbeatLineReader(nil, commonsse.NewLineReader(source, 256), time.Second)
	defer h.Close()
	line, err := h.Next()
	require.NoError(t, err)
	require.True(t, line.Oversized)
	reads := source.calls.Load()
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, reads, source.calls.Load(), "the worker must wait for demand before reading again")
	actual, err := io.ReadAll(line.Large)
	require.NoError(t, err)
	require.Equal(t, payload, string(actual))
	blank, err := h.Next()
	require.NoError(t, err)
	require.Equal(t, commonsse.LineKindBlank, blank.Kind)
	terminal, err := h.Next()
	require.NoError(t, err)
	require.Equal(t, "data: [DONE]", terminal.Text())
	h.Close()
	select {
	case <-h.workerStopped:
	case <-time.After(time.Second):
		t.Fatal("idle worker did not stop")
	}
}

// TestHeartbeatDemandCloseUnblocksWaitingNext verifies Close releases the consumer and closing the owned body releases the producer.
func TestHeartbeatDemandCloseUnblocksWaitingNext(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	h := NewHeartbeatLineReader(nil, commonsse.NewLineReader(reader, 256), time.Second)
	defer h.Close()
	result := make(chan error, 1)
	go func() { _, err := h.Next(); result <- err }()
	h.Close()
	select {
	case err := <-result:
		require.ErrorIs(t, err, io.EOF)
	case <-time.After(time.Second):
		t.Fatal("Next did not stop")
	}
	require.NoError(t, reader.Close())
	select {
	case <-h.workerStopped:
	case <-time.After(time.Second):
		t.Fatal("worker retained a closed upstream")
	}
}

// BenchmarkHeartbeatDemand measures line transport allocations separately from gateway E2E evidence.
func BenchmarkHeartbeatDemand(b *testing.B) {
	input := strings.Repeat("data: {\"choices\":[]}\n\n", 1024)
	b.ReportAllocs()
	for b.Loop() {
		h := NewHeartbeatLineReader(nil, commonsse.NewLineReader(strings.NewReader(input), commonsse.DefaultLineBufferSize), time.Second)
		for {
			_, err := h.Next()
			if err == io.EOF {
				break
			}
			require.NoError(b, err)
		}
		h.Close()
		<-h.workerStopped
	}
}
