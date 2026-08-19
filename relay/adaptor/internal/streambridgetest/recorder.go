package streambridgetest

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// Recorder records StreamRewriteHandler calls for adapter stream bridge tests.
type Recorder struct {
	Deltas            []string
	Chunks            []*openai_compatible.ChatCompletionsStreamResponse
	UpstreamDoneCount int
	DoneCount         int
	UsageSet          bool
	Usage             *model.Usage
	HandleChunkDone   bool
}

// HandleChunk records each normalized chat-completion stream chunk and returns
// handled=true so the caller suppresses raw SSE forwarding. It returns the
// configured HandleChunkDone value for doneRendered.
func (r *Recorder) HandleChunk(_ *gin.Context, chunk *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	r.Chunks = append(r.Chunks, chunk)
	for _, ch := range chunk.Choices {
		r.Deltas = append(r.Deltas, ch.Delta.StringContent())
	}
	return true, r.HandleChunkDone
}

// HandleUpstreamDone records that the upstream emitted a terminal DONE frame.
func (r *Recorder) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	r.UpstreamDoneCount++
	return true, false
}

// HandleDone records finalization and reports that terminal events were emitted.
func (r *Recorder) HandleDone(_ *gin.Context) (bool, bool) {
	r.DoneCount++
	return true, true
}

// FinalizeUsage records the final usage pointer provided by the stream bridge.
func (r *Recorder) FinalizeUsage(usage *model.Usage) {
	r.UsageSet = true
	r.Usage = usage
}

// JoinedDeltas returns all recorded content deltas concatenated in order.
func (r *Recorder) JoinedDeltas() string {
	return strings.Join(r.Deltas, "")
}
