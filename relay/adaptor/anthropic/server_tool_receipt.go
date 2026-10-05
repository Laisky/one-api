package anthropic

import (
	"bytes"
	"encoding/json"
	"maps"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
)

// serverToolTally accumulates provider-executed tool invocations for one
// upstream message. receipts holds the authoritative usage.server_tool_use
// counters (cumulative, merged by maximum so repeated or out-of-order stream
// receipts never add up twice and an omitted counter never erases an earlier
// one). blocks counts observed server-side invocation content blocks and is
// used only for tools whose receipt counter never appeared, so a missing receipt
// is not silently read as free usage.
type serverToolTally struct {
	receipts map[string]int
	blocks   map[string]int
}

// serverToolCounterName maps a usage.server_tool_use counter to its canonical
// capability. Parameters: key is the counter field name. Returns: the canonical
// name, or "" for fields that are not invocation counters.
func serverToolCounterName(key string) string {
	switch key {
	case "web_search_requests":
		return ToolTypeWebSearch
	case "web_fetch_requests":
		return ToolTypeWebFetch
	case "code_execution_requests":
		return ToolTypeCodeExecution
	}
	if name, ok := strings.CutSuffix(strings.ToLower(strings.TrimSpace(key)), "_requests"); ok && name != "" {
		return name
	}
	return ""
}

// serverToolBlockName maps a response content block to the canonical capability
// it invoked. Parameters: blockType and name come from the content block.
// Returns: the canonical name, or "" for blocks that are not provider-side calls.
func serverToolBlockName(blockType, name string) string {
	switch blockType {
	case "mcp_tool_use":
		return ToolTypeMCPConnector
	case "server_tool_use":
	default:
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case normalized == "":
		return ""
	case normalized == ToolTypeWebSearch, normalized == ToolTypeWebFetch:
		return normalized
	case normalized == ToolTypeCodeExecution, normalized == "bash_code_execution", normalized == "text_editor_code_execution":
		return ToolTypeCodeExecution
	case strings.HasPrefix(normalized, "tool_search_tool_"):
		return ToolTypeToolSearch
	default:
		return normalized
	}
}

// parseServerToolUse validates a usage.server_tool_use object. Parameters: raw
// is the JSON value (absent or null yields no counters). Returns: canonical
// counters reported by this receipt, or an error for negative or non-integer
// invocation counters.
func parseServerToolUse(raw json.RawMessage) (map[string]int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, errors.Wrap(err, "decode Claude server_tool_use receipt")
	}
	counters := make(map[string]int, len(fields))
	for key, value := range fields {
		name := serverToolCounterName(key)
		if name == "" || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		var count int
		if err := json.Unmarshal(value, &count); err != nil {
			return nil, errors.Wrapf(err, "decode Claude server_tool_use counter %s", key)
		}
		if count < 0 {
			return nil, errors.Errorf("Claude server_tool_use counter %s is negative", key)
		}
		counters[name] = max(counters[name], count)
	}
	return counters, nil
}

// withReceipts returns a copy of the tally with receipt counters merged by
// maximum. Parameters: update holds one receipt's canonical counters. Returns:
// the merged tally; the receiver is never mutated so callers keep atomic merges.
func (t serverToolTally) withReceipts(update map[string]int) serverToolTally {
	if len(update) == 0 {
		return t
	}
	merged := maps.Clone(t.receipts)
	if merged == nil {
		merged = make(map[string]int, len(update))
	}
	for name, count := range update {
		if previous, ok := merged[name]; !ok || count > previous {
			merged[name] = count
		}
	}
	return serverToolTally{receipts: merged, blocks: t.blocks}
}

// observeBlock counts one provider-side invocation content block. Parameters:
// blockType and name identify the block. Returns: nothing.
func (t *serverToolTally) observeBlock(blockType, name string) {
	canonical := serverToolBlockName(blockType, name)
	if canonical == "" {
		return
	}
	if t.blocks == nil {
		t.blocks = make(map[string]int)
	}
	t.blocks[canonical]++
}

// counts returns the billable invocation count per canonical capability:
// authoritative receipts when reported, otherwise observed invocation blocks.
func (t serverToolTally) counts() map[string]int {
	out := make(map[string]int, len(t.receipts)+len(t.blocks))
	for name, count := range t.receipts {
		if count > 0 {
			out[name] = count
		}
	}
	for name, count := range t.blocks {
		if _, reported := t.receipts[name]; reported || count <= 0 {
			continue
		}
		out[name] = count
	}
	return out
}

// RecordServerToolInvocations adds one upstream message's server-tool counts to
// the request-scoped ctxkey.ToolInvocationCounts consumed by the shared tooling
// billing. Separate upstream messages (for example MCP loop rounds) add up.
// Parameters: c is the request context and counts holds canonical counts.
// Returns: nothing.
func RecordServerToolInvocations(c *gin.Context, counts map[string]int) {
	if c == nil || len(counts) == 0 {
		return
	}
	merged := make(map[string]int, len(counts))
	if raw, ok := c.Get(ctxkey.ToolInvocationCounts); ok {
		mergeInvocationCounts(merged, raw)
	}
	recorded := false
	for name, count := range counts {
		if count <= 0 {
			continue
		}
		merged[name] += count
		recorded = true
	}
	if !recorded {
		return
	}
	c.Set(ctxkey.ToolInvocationCounts, merged)
	lg := gmw.GetLogger(c)
	lg.Debug("recorded Claude server tool invocations", zap.Any("counts", counts))
}

// RecordServerToolUseFromJSON extracts server-tool usage from one complete,
// non-streaming Claude message body and records it. Parameters: c is the request
// context and body is the raw message. Returns: an error when the receipt is
// malformed; observed invocation blocks are still recorded in that case.
func RecordServerToolUseFromJSON(c *gin.Context, body []byte) error {
	var message struct {
		Usage struct {
			ServerToolUse json.RawMessage `json:"server_tool_use"`
		} `json:"usage"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(body, &message); err != nil {
		return errors.Wrap(err, "decode Claude message for server tool usage")
	}
	// A malformed receipt still leaves the observed invocation blocks billable.
	counters, err := parseServerToolUse(message.Usage.ServerToolUse)
	tally := serverToolTally{}.withReceipts(counters)
	tally.observeContent(message.Content)
	RecordServerToolInvocations(c, tally.counts())
	return err
}

// observeContent counts invocation blocks in a native content array. Content
// that is not an array of objects is ignored because counting is only a
// fallback for a missing receipt. Parameters: raw is the content JSON.
func (t *serverToolTally) observeContent(raw json.RawMessage) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	var blocks []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return
	}
	for _, block := range blocks {
		t.observeBlock(block.Type, block.Name)
	}
}

// mergeInvocationCounts accumulates an existing counter map of any supported
// numeric representation. Parameters: dst receives counts and raw is the stored value.
func mergeInvocationCounts(dst map[string]int, raw any) {
	switch typed := raw.(type) {
	case map[string]int:
		for name, count := range typed {
			dst[name] += count
		}
	case map[string]int64:
		for name, count := range typed {
			dst[name] += int(count)
		}
	case map[string]float64:
		for name, count := range typed {
			dst[name] += int(count)
		}
	case map[string]any:
		for name, value := range typed {
			switch count := value.(type) {
			case int:
				dst[name] += count
			case int64:
				dst[name] += int(count)
			case float64:
				dst[name] += int(count)
			}
		}
	}
}
