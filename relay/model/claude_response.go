package model

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ClaudeResponse is the non-streaming Anthropic Messages API Message object that
// converted (non-Anthropic) upstream answers are rendered into.
//
// stop_reason and stop_sequence are always serialized: the Messages contract
// marks them present-but-nullable, so StopSequence encodes as null rather than
// disappearing when no stop sequence matched.
type ClaudeResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	Model        string          `json:"model"`
	Content      []ClaudeContent `json:"content"`
	StopReason   string          `json:"stop_reason"`
	StopSequence *string         `json:"stop_sequence"`
	Usage        ClaudeUsage     `json:"usage"`
}

// ClaudeContent is one content block of a Messages API response. Its JSON shape
// depends on Type (see MarshalJSON); decoding accepts every field.
type ClaudeContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
}

// claudeTextBlockWire is the text block wire shape; text is always present.
type claudeTextBlockWire struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// claudeThinkingBlockWire is the thinking block wire shape; both thinking and
// signature are always present (an unsigned block carries an empty signature).
type claudeThinkingBlockWire struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

// claudeToolUseBlockWire is the tool_use block wire shape; input is always a
// JSON value, {} when the tool takes no arguments.
type claudeToolUseBlockWire struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// claudeEmptyToolInput is the canonical input of an argument-less tool call.
var claudeEmptyToolInput = json.RawMessage(`{}`)

// MarshalJSON renders the block with the field set the Messages contract
// requires for its type: text blocks always carry "text" (even when empty),
// thinking blocks always carry "thinking" and "signature", and tool_use blocks
// always carry "id", "name", and an "input" value ({} when absent or not valid
// JSON). Other block types keep the generic omitempty encoding. It returns the
// encoded bytes or the marshaling error.
func (c ClaudeContent) MarshalJSON() ([]byte, error) {
	var (
		out []byte
		err error
	)
	switch c.Type {
	case "text":
		out, err = json.Marshal(claudeTextBlockWire{Type: c.Type, Text: c.Text})
	case "thinking":
		out, err = json.Marshal(claudeThinkingBlockWire{Type: c.Type, Thinking: c.Thinking, Signature: c.Signature})
	case "tool_use":
		out, err = json.Marshal(claudeToolUseBlockWire{Type: c.Type, ID: c.ID, Name: c.Name, Input: ClaudeToolInputOrEmpty(c.Input)})
	default:
		type plainClaudeContent ClaudeContent
		out, err = json.Marshal(plainClaudeContent(c))
	}
	if err != nil {
		return nil, errors.Wrapf(err, "marshal Claude %q content block", c.Type)
	}
	return out, nil
}

// ClaudeToolInputOrEmpty returns raw when it is a non-null, syntactically valid
// JSON value and the canonical empty object {} otherwise, so a tool_use block
// never omits its input or carries bytes that would break the enclosing JSON.
func ClaudeToolInputOrEmpty(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || !json.Valid(trimmed) {
		return claudeEmptyToolInput
	}
	return json.RawMessage(trimmed)
}

// ClaudeUsage is the Messages API usage object. input_tokens and output_tokens
// are required; the cache counters are optional and omitted when zero.
type ClaudeUsage struct {
	InputTokens              int                  `json:"input_tokens"`
	OutputTokens             int                  `json:"output_tokens"`
	CacheReadInputTokens     int                  `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int                  `json:"cache_creation_input_tokens,omitempty"`
	CacheCreation            *ClaudeCacheCreation `json:"cache_creation,omitempty"`
}

// ClaudeCacheCreation stores Claude cache creation token buckets for different TTL windows.
type ClaudeCacheCreation struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens,omitempty"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens,omitempty"`
}

// PromptIncludesCacheBuckets reports whether u.PromptTokens already counts the
// cache-read bucket (PromptTokensDetails.CachedTokens) and the cache-write
// buckets (CacheWrite5mTokens + CacheWrite1hTokens), as OpenAI-family receipts
// (Chat prompt_tokens, Responses input_tokens, Gemini promptTokenCount) do.
//
// It applies the interpretation quota.Compute settles with, so presented and
// billed prompt totals agree: usage of a Claude model, or usage whose cache
// buckets exceed PromptTokens, follows Anthropic semantics where PromptTokens
// excludes the cache buckets. Parameters: modelName is the model the usage is
// billed under. Returns: true for OpenAI semantics, and true for a nil receipt.
func (u *Usage) PromptIncludesCacheBuckets(modelName string) bool {
	if u == nil {
		return true
	}
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = max(u.PromptTokensDetails.CachedTokens, 0)
	}
	writes := max(u.CacheWrite5mTokens, 0) + max(u.CacheWrite1hTokens, 0)
	return !containsClaudeASCIIFold(modelName) && cached <= u.PromptTokens && writes <= u.PromptTokens
}

// containsClaudeASCIIFold reports whether modelName contains "claude" under
// ASCII case folding, matching the Claude-model test quota.Compute applies.
// Parameters: modelName is the billed model. Returns: true for Claude models.
func containsClaudeASCIIFold(modelName string) bool {
	const token = "claude"
	for start := 0; start+len(token) <= len(modelName); start++ {
		matched := true
		for offset := range len(token) {
			b := modelName[start+offset]
			if 'A' <= b && b <= 'Z' {
				b += 'a' - 'A'
			}
			if b != token[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// ClaudeUsageFromUsage presents a canonical usage receipt as the Anthropic
// Messages usage object. Anthropic input_tokens excludes cache reads and cache
// writes, and Claude clients (Claude Code's context window, auto-compaction
// trigger, and /context display) size the prompt as input_tokens +
// cache_read_input_tokens + cache_creation_input_tokens.
//
// For OpenAI-semantics usage (see PromptIncludesCacheBuckets) cache_read is the
// cached count, cache_creation is the 5m + 1h writes with their TTL split, and
// input_tokens is the remainder, clamped like quota.Compute (writes beyond the
// uncached remainder are trimmed 1h first) so the three buckets sum to
// PromptTokens. Usage already in Anthropic semantics keeps PromptTokens as
// input_tokens. 12h cache writes have no Anthropic bucket and stay in
// input_tokens. Output is unchanged. Parameters: usage is the receipt (nil
// yields zero usage) and modelName the model it is billed under. Returns: the
// Claude usage object.
func ClaudeUsageFromUsage(usage *Usage, modelName string) ClaudeUsage {
	if usage == nil {
		return ClaudeUsage{}
	}
	read := 0
	if usage.PromptTokensDetails != nil {
		read = max(usage.PromptTokensDetails.CachedTokens, 0)
	}
	write5m, write1h := max(usage.CacheWrite5mTokens, 0), max(usage.CacheWrite1hTokens, 0)
	input := max(usage.PromptTokens, 0)
	if usage.PromptIncludesCacheBuckets(modelName) {
		input -= read
		if excess := write5m + write1h - input; excess > 0 {
			trimmed := min(write1h, excess)
			write1h -= trimmed
			write5m -= min(write5m, excess-trimmed)
		}
		input -= write5m + write1h
	}
	out := ClaudeUsage{
		InputTokens:              input,
		OutputTokens:             usage.CompletionTokens,
		CacheReadInputTokens:     read,
		CacheCreationInputTokens: write5m + write1h,
	}
	if write5m > 0 || write1h > 0 {
		out.CacheCreation = &ClaudeCacheCreation{Ephemeral5mInputTokens: write5m, Ephemeral1hInputTokens: write1h}
	}
	return out
}

// ConvertedClaudeBillingUsage returns the server-only settlement receipt of a
// Claude Messages response converted from an OpenAI-semantics upstream: the
// prompt, completion, cache-read, and 5m/1h cache-write counters of usage, with
// TotalTokens = prompt + completion. These are exactly the counters the
// controller historically re-parsed from the converted body, so settling on the
// receipt keeps billing unchanged while the body moves to Anthropic semantics.
// Parameters: usage is the measured upstream receipt. Returns: a fresh receipt,
// or nil when usage reports no positive prompt or completion count (the
// controller then keeps its estimate fallback).
func ConvertedClaudeBillingUsage(usage *Usage) *Usage {
	if usage == nil || (usage.PromptTokens <= 0 && usage.CompletionTokens <= 0) {
		return nil
	}
	receipt := &Usage{
		PromptTokens:       usage.PromptTokens,
		CompletionTokens:   usage.CompletionTokens,
		TotalTokens:        usage.PromptTokens + usage.CompletionTokens,
		CacheWrite5mTokens: max(usage.CacheWrite5mTokens, 0),
		CacheWrite1hTokens: max(usage.CacheWrite1hTokens, 0),
	}
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
		receipt.PromptTokensDetails = &UsagePromptTokensDetails{CachedTokens: usage.PromptTokensDetails.CachedTokens}
	}
	return receipt
}
