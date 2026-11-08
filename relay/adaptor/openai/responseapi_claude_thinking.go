package openai

import (
	"github.com/Laisky/one-api/relay/model"
)

// ResponseClaudeThinkingEnvelopeStats counts gateway Claude envelopes found in
// Responses reasoning items, for diagnostics only.
type ResponseClaudeThinkingEnvelopeStats struct {
	// Valid counts envelopes that decode into a replayable Claude block.
	Valid int
	// Ignored counts items that claim the gateway prefix but fail validation;
	// they fall back to the summary-text replay used for foreign reasoning.
	Ignored int
}

// responseReasoningClaudeThinkingBlock decodes the gateway Claude envelope of a
// Responses reasoning item. The item parameter is one untrusted input item. It
// returns the block and true only for a valid envelope; foreign provider state
// (for example OpenAI encrypted reasoning) and malformed or oversized envelopes
// return false so the caller keeps the established summary-text fallback and
// never pairs a signature with text that did not travel inside the envelope.
func responseReasoningClaudeThinkingBlock(item map[string]any) (model.ClaudeThinkingBlock, bool) {
	envelope, _ := item["encrypted_content"].(string)
	if !model.IsClaudeThinkingEnvelope(envelope) {
		return model.ClaudeThinkingBlock{}, false
	}
	block, err := model.DecodeClaudeThinkingEnvelope(envelope)
	if err != nil {
		// Deliberate degradation: InspectResponseClaudeThinkingEnvelopes lets the
		// caller log how many envelopes were ignored without logging content.
		return model.ClaudeThinkingBlock{}, false
	}
	return block, true
}

// InspectResponseClaudeThinkingEnvelopes counts valid and ignored gateway Claude
// envelopes in input without changing it. The input parameter is the Responses
// input list; the return value is used only for debug diagnostics.
func InspectResponseClaudeThinkingEnvelopes(input []any) ResponseClaudeThinkingEnvelopeStats {
	var stats ResponseClaudeThinkingEnvelopeStats
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := item["type"].(string); kind != "reasoning" {
			continue
		}
		envelope, _ := item["encrypted_content"].(string)
		if !model.IsClaudeThinkingEnvelope(envelope) {
			continue
		}
		if _, ok := responseReasoningClaudeThinkingBlock(item); ok {
			stats.Valid++
		} else {
			stats.Ignored++
		}
	}
	return stats
}

// attachClaudeThinkingBlocks appends replayable blocks to an assistant message
// in arrival order. Parameters: message is the lowered assistant message and
// blocks the staged blocks. It returns nothing; empty input is a no-op.
func attachClaudeThinkingBlocks(message *model.Message, blocks []model.ClaudeThinkingBlock) {
	if message == nil || len(blocks) == 0 {
		return
	}
	message.ClaudeThinkingBlocks = append(message.ClaudeThinkingBlocks, blocks...)
}

// StripResponseGatewayClaudeReasoning removes reasoning items whose
// encrypted_content carries a gateway Claude thinking envelope from input. The
// envelope is only meaningful to this gateway's Claude converter; a native
// Responses provider would treat it as invalid encrypted reasoning, which can
// happen when a client moves a conversation from a Claude fallback channel to
// a native channel. The input parameter is modified in place. It returns the
// number of removed items.
func StripResponseGatewayClaudeReasoning(input *ResponseAPIInput) int {
	if input == nil || len(*input) == 0 {
		return 0
	}
	kept := (*input)[:0:0]
	removed := 0
	for _, raw := range *input {
		if item, ok := raw.(map[string]any); ok {
			kind, _ := item["type"].(string)
			envelope, _ := item["encrypted_content"].(string)
			if kind == "reasoning" && model.IsClaudeThinkingEnvelope(envelope) {
				removed++
				continue
			}
		}
		kept = append(kept, raw)
	}
	if removed > 0 {
		*input = kept
	}
	return removed
}
