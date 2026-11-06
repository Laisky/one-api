package controller

import (
	"slices"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// responseIncludeReasoningEncryptedContent is the Responses include value that
// asks for replayable reasoning state on every reasoning item.
const responseIncludeReasoningEncryptedContent = "reasoning.encrypted_content"

// responseFallbackReplaysClaudeThinking reports whether the fallback Chat
// request is lowered by the Claude Messages converter, the only consumer of
// gateway thinking blocks. The meta parameter carries the resolved upstream;
// the return value is false for every non-Claude Chat upstream.
func responseFallbackReplaysClaudeThinking(meta *metalib.Meta) bool {
	if meta == nil {
		return false
	}
	modelName := strings.ToLower(strings.TrimSpace(meta.ActualModelName))
	switch meta.APIType {
	case apitype.Anthropic:
		return true
	case apitype.AwsClaude:
		// Bedrock hosts several families; only Claude ids and ARNs use the
		// Claude converter, the others marshal Chat messages directly.
		return strings.Contains(modelName, "claude")
	case apitype.VertexAI:
		return strings.HasPrefix(modelName, "claude")
	case apitype.Azure:
		return meta.AzureTargetsAnthropic()
	default:
		return false
	}
}

// applyClaudeThinkingReplayPolicy strips gateway Claude thinking blocks from a
// lowered fallback request unless the upstream replays them, so non-Claude
// upstreams never receive Claude provider state. Parameters: c is the request
// context, meta the resolved upstream, input the Responses input it was lowered
// from (for diagnostics) and request the lowered Chat request, mutated in place.
func applyClaudeThinkingReplayPolicy(c *gin.Context, meta *metalib.Meta, input []any, request *relaymodel.GeneralOpenAIRequest) {
	if request == nil {
		return
	}
	lg := gmw.GetLogger(c)
	if config.DebugEnabled {
		if stats := openai.InspectResponseClaudeThinkingEnvelopes(input); stats.Valid+stats.Ignored > 0 {
			lg.Debug("inspected Claude thinking envelopes in Responses input",
				zap.Int("valid_envelopes", stats.Valid),
				zap.Int("ignored_envelopes", stats.Ignored),
			)
		}
	}
	if responseFallbackReplaysClaudeThinking(meta) {
		return
	}
	var stripped int
	request.Messages, stripped = stripClaudeThinkingBlocks(request.Messages)
	if stripped > 0 {
		lg.Debug("stripped Claude thinking blocks for non-Claude fallback upstream",
			zap.Int("stripped_blocks", stripped),
			zap.Int("api_type", meta.APIType),
		)
	}
}

// stripClaudeThinkingBlocks returns messages without gateway Claude thinking
// blocks, copying the slice only when a message carries blocks so caller-owned
// data is never mutated. It returns the messages and the removed block count.
func stripClaudeThinkingBlocks(messages []relaymodel.Message) ([]relaymodel.Message, int) {
	removed := 0
	var out []relaymodel.Message
	for i := range messages {
		if len(messages[i].ClaudeThinkingBlocks) == 0 {
			continue
		}
		if out == nil {
			out = slices.Clone(messages)
		}
		removed += len(out[i].ClaudeThinkingBlocks)
		out[i].ClaudeThinkingBlocks = nil
	}
	if out == nil {
		return messages, 0
	}
	return out, removed
}

// responseIncludesReasoningEncryptedContent reports whether the client asked for
// encrypted reasoning state via include. A nil request returns false.
func responseIncludesReasoningEncryptedContent(request *openai.ResponseAPIRequest) bool {
	return request != nil && slices.Contains(request.Include, responseIncludeReasoningEncryptedContent)
}

// newClaudeThinkingReasoningItem renders one Claude thinking block as a
// completed Responses reasoning item. Parameters: lg reports envelope failures,
// id is the item ID, block the replayable block and summary its display text
// (empty for omitted display or redacted thinking, giving an empty summary).
// The returned item always carries the gateway envelope when encoding succeeds;
// clientVisibleResponseOutput removes it when the client did not ask for it.
func newClaudeThinkingReasoningItem(lg glog.Logger, id string, block relaymodel.ClaudeThinkingBlock, summary string) openai.OutputItem {
	item := openai.OutputItem{Id: id, Type: "reasoning", Status: "completed", Summary: make([]openai.OutputContent, 0, 1)}
	if summary != "" {
		item.Summary = append(item.Summary, openai.OutputContent{Type: "summary_text", Text: summary})
	}
	envelope, err := relaymodel.EncodeClaudeThinkingEnvelope(block)
	if err != nil {
		// The reasoning item still renders; without an envelope the turn replays
		// through the established summary-text fallback.
		lg.Warn("encode Claude thinking envelope failed", zap.String("block_type", block.Type), zap.Error(err))
		return item
	}
	item.EncryptedContent = envelope
	return item
}

// newClaudeThinkingReasoningID mints a Responses reasoning item ID.
func newClaudeThinkingReasoningID() string {
	return "rs_" + random.GetRandomString(16)
}

// clientVisibleResponseOutput returns outputs as the client may see them: when
// include asks for encrypted reasoning the slice is returned unchanged,
// otherwise gateway Claude envelopes are removed from reasoning items (state
// persistence keeps them). Parameters: outputs are the full items and include
// reports the client's opt-in. A copy is made only when an item changes.
func clientVisibleResponseOutput(outputs []openai.OutputItem, include bool) []openai.OutputItem {
	if include {
		return outputs
	}
	var visible []openai.OutputItem
	for i := range outputs {
		if outputs[i].Type != "reasoning" || !relaymodel.IsClaudeThinkingEnvelope(outputs[i].EncryptedContent) {
			continue
		}
		if visible == nil {
			visible = slices.Clone(outputs)
		}
		visible[i].EncryptedContent = ""
	}
	if visible == nil {
		return outputs
	}
	return visible
}

// clientVisibleOutputItem applies clientVisibleResponseOutput to one item.
// The item parameter is the full item and include the client's opt-in.
func clientVisibleOutputItem(item openai.OutputItem, include bool) openai.OutputItem {
	if !include && item.Type == "reasoning" && relaymodel.IsClaudeThinkingEnvelope(item.EncryptedContent) {
		item.EncryptedContent = ""
	}
	return item
}
