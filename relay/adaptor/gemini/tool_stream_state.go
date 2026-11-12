package gemini

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

const geminiCompleteToolStateKey = "gemini_complete_tool_stream_state"

const vertexPartPositionsKey = "gemini_vertex_part_positions"

// EnableVertexPartPositions enables gateway order metadata for this trusted Vertex request.
func EnableVertexPartPositions(c *gin.Context) { c.Set(vertexPartPositionsKey, true) }

// geminiVertexPartPositions reports whether this request is using the native Vertex adapter.
func geminiVertexPartPositions(c *gin.Context) bool {
	return c != nil && c.GetBool(vertexPartPositionsKey)
}

// geminiBufferedPartPositions preserves positions for Vertex calls or actual signed/thought native parts.
func geminiBufferedPartPositions(c *gin.Context, candidate *ChatCandidate) bool {
	for _, part := range candidate.Content.Parts {
		if part.ThoughtSignature != "" || part.Thought || part.GatewayPartIndex != nil {
			return true
		}
		if geminiVertexPartPositions(c) && part.FunctionCall != nil {
			return true
		}
	}
	return false
}

// geminiCompleteToolState returns request-local complete-call counters by candidate.
func geminiCompleteToolState(c *gin.Context) map[int64]int {
	if value, ok := c.Get(geminiCompleteToolStateKey); ok {
		if counts, ok := value.(map[int64]int); ok {
			return counts
		}
	}
	counts := make(map[int64]int)
	c.Set(geminiCompleteToolStateKey, counts)
	return counts
}

// nextGeminiCompleteToolIndex allocates one contiguous index for a complete call in a candidate.
func nextGeminiCompleteToolIndex(c *gin.Context, candidate int64) int {
	counts := geminiCompleteToolState(c)
	index := counts[candidate]
	counts[candidate] = index + 1
	return index
}

// geminiCompleteToolCount returns the number of complete calls emitted for one candidate.
func geminiCompleteToolCount(c *gin.Context, candidate int64) int {
	return geminiCompleteToolState(c)[candidate]
}

// geminiChatFinishReason maps the native terminal reason to the public Chat contract.
func geminiChatFinishReason(reason string, hasTools bool) string {
	switch reason {
	case "STOP":
		if hasTools {
			return "tool_calls"
		}
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "content_filter"
	default:
		return "stop"
	}
}

// unsupportedGeminiFunctionFragments detects a protocol that this complete-call relay cannot represent.
// Parameters: response is a decoded native response. Returns true when incremental argument fields occur.
func unsupportedGeminiFunctionFragments(response *ChatResponse) bool {
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if call := part.FunctionCall; call != nil {
				if call.WillContinue != nil && *call.WillContinue {
					return true
				}
				if len(call.PartialArgs) > 0 {
					var fragments []json.RawMessage
					if err := json.Unmarshal(call.PartialArgs, &fragments); err != nil || len(fragments) > 0 {
						return true
					}
				}
			}
		}
	}
	return false
}

// geminiPartStreamState tracks original projected part offsets independently for each candidate.
type geminiPartStreamState struct {
	next       map[int64]int
	positioned map[int64]bool
}

// assignGeminiStreamPartPositions records cumulative native part boundaries for tool-capable streams.
// Ordinary no-tool text keeps its scalar Chat shape. Late metadata without prior tool context
// cannot recover earlier scalar boundaries; incomplete replay metadata is rejected rather than guessed.
func assignGeminiStreamPartPositions(c *gin.Context, response *ChatResponse) {
	const key = "gemini_native_part_stream_positions"
	var state *geminiPartStreamState
	if value, exists := c.Get(key); exists {
		state, _ = value.(*geminiPartStreamState)
	}
	if state == nil {
		state = &geminiPartStreamState{next: map[int64]int{}, positioned: map[int64]bool{}}
		c.Set(key, state)
	}
	toolCapable := false
	if value, exists := c.Get(ctxkey.ConvertedRequest); exists {
		if request, ok := value.(*ChatRequest); ok {
			toolCapable = geminiVertexPartPositions(c) && len(request.Tools) > 0
		}
	}
	for i := range response.Candidates {
		candidate := &response.Candidates[i]
		enabled := toolCapable || state.positioned[candidate.Index]
		for _, part := range candidate.Content.Parts {
			if (geminiVertexPartPositions(c) && part.FunctionCall != nil) || part.ThoughtSignature != "" || part.Thought {
				enabled = true
			}
		}
		state.positioned[candidate.Index] = enabled
		for j := range candidate.Content.Parts {
			part := &candidate.Content.Parts[j]
			index := state.next[candidate.Index]
			state.next[candidate.Index] = index + 1
			if enabled {
				part.GatewayPartIndex = &index
			}
		}
	}
}

// geminiPartExtraContent projects documented Google metadata plus optional gateway ordering metadata.
func geminiPartExtraContent(part Part, index int, positioned bool) *model.ToolExtraContent {
	extra := model.GooglePartMetadata(part.ThoughtSignature, part.Thought)
	if part.GatewayPartIndex != nil {
		extra = model.WithGeminiPartIndex(extra, *part.GatewayPartIndex)
	} else if positioned {
		extra = model.WithGeminiPartIndex(extra, index)
	}
	if extra != nil && part.Text == "" && !part.TextPresent && part.FunctionCall == nil && part.InlineData == nil && part.FileData == nil {
		extra = model.WithGeminiTextPresent(extra, false)
	}
	return extra
}

// geminiCompleteArguments normalizes an absent protobuf Struct to the empty object it represents.
func geminiCompleteArguments(arguments any) any {
	if arguments == nil {
		return map[string]any{}
	}
	if object, ok := arguments.(map[string]any); ok && object == nil {
		return map[string]any{}
	}
	return arguments
}
