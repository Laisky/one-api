package openai

import (
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// recordWebSearchPreviewInvocation infers at least one web search tool call for
// OpenAI chat completion requests using search-preview models when upstream
// responses omit invocation counts. It skips recording when upstream already
// provided web search counts to avoid double billing.
func recordWebSearchPreviewInvocation(c *gin.Context, metaInfo *meta.Meta) {
	if c == nil || metaInfo == nil {
		return
	}

	if metaInfo.ChannelType != channeltype.OpenAI || metaInfo.Mode != relaymode.ChatCompletions {
		return
	}

	modelName := strings.ToLower(strings.TrimSpace(metaInfo.ActualModelName))
	if modelName == "" || !isWebSearchModel(modelName) {
		return
	}

	if hasRecordedWebSearchCount(c) {
		return
	}

	toolName := "web_search_preview_non_reasoning"
	if isModelSupportedReasoning(modelName) {
		toolName = "web_search_preview_reasoning"
	}

	counts := normalizeToolInvocationCounts(c)
	counts[toolName]++
	c.Set(ctxkey.ToolInvocationCounts, counts)

	gmw.GetLogger(c).Debug("recorded implicit web search preview call",
		zap.String("model", modelName),
		zap.String("tool", toolName),
		zap.Int("count", counts[toolName]),
	)
}

// hasRecordedWebSearchCount reports whether the context already carries any web
// search invocation counters, either from explicit upstream metadata or prior
// bookkeeping.
func hasRecordedWebSearchCount(c *gin.Context) bool {
	if raw, ok := c.Get(ctxkey.WebSearchCallCount); ok {
		if intFromAny(raw) > 0 {
			return true
		}
	}

	if raw, ok := c.Get(ctxkey.ToolInvocationCounts); ok {
		existing := normalizeToolInvocationCountsFromRaw(raw)
		if existing["web_search"] > 0 || existing["web_search_preview_reasoning"] > 0 || existing["web_search_preview_non_reasoning"] > 0 {
			return true
		}
	}

	return false
}

// normalizeToolInvocationCounts merges any pre-existing invocation counters on
// the context into a lowercase map so additional counts can be appended safely.
func normalizeToolInvocationCounts(c *gin.Context) map[string]int {
	counts := make(map[string]int)
	if raw, ok := c.Get(ctxkey.ToolInvocationCounts); ok {
		mergeToolInvocationCounts(counts, raw)
	}
	return counts
}

// normalizeToolInvocationCountsFromRaw converts a raw invocation counter map to
// a normalized lowercase map without mutating the source.
func normalizeToolInvocationCountsFromRaw(raw any) map[string]int {
	counts := make(map[string]int)
	mergeToolInvocationCounts(counts, raw)
	return counts
}

// mergeToolInvocationCounts accumulates invocation counters of varying numeric
// types into the destination map, lowercasing tool identifiers.
func mergeToolInvocationCounts(dst map[string]int, raw any) {
	switch typed := raw.(type) {
	case map[string]int:
		for name, count := range typed {
			dst[strings.ToLower(strings.TrimSpace(name))] += count
		}
	case map[string]int64:
		for name, count := range typed {
			dst[strings.ToLower(strings.TrimSpace(name))] += int(count)
		}
	case map[string]float64:
		for name, count := range typed {
			dst[strings.ToLower(strings.TrimSpace(name))] += int(count)
		}
	case map[string]any:
		for name, count := range typed {
			dst[strings.ToLower(strings.TrimSpace(name))] += intFromAny(count)
		}
	}
}

// intFromAny safely converts supported numeric representations to int.
func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}
