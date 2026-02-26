package openai

import (
	"fmt"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

func generalToolSummary(tools []model.Tool) (bool, []string) {
	if len(tools) == 0 {
		return false, nil
	}
	hasWebSearch := false
	types := make([]string, 0, len(tools))
	for _, tool := range tools {
		typeName := strings.ToLower(strings.TrimSpace(tool.Type))
		if typeName == "" && tool.Function != nil {
			typeName = "function"
		}
		if typeName == "" {
			typeName = "unknown"
		}
		types = append(types, typeName)
		if typeName == "web_search" {
			hasWebSearch = true
		}
	}
	return hasWebSearch, types
}

func responseAPIToolSummary(tools []ResponseAPITool) (bool, []string) {
	if len(tools) == 0 {
		return false, nil
	}
	hasWebSearch := false
	types := make([]string, 0, len(tools))
	for _, tool := range tools {
		typeName := strings.ToLower(strings.TrimSpace(tool.Type))
		if typeName == "" {
			typeName = "unknown"
		}
		types = append(types, typeName)
		if strings.HasPrefix(typeName, "web_search") {
			hasWebSearch = true
		}
	}
	return hasWebSearch, types
}

// responseAPIInputDiagnostics summarizes malformed Response API input items for safe DEBUG logging.
// Parameters: input is the Response API input array.
// Returns: counts and sample indices for malformed function_call/function_call_output items.
type responseAPIInputDiagnostics struct {
	FunctionCallOutputMissingOutput int
	FunctionCallOutputMissingCallID int
	FunctionCallMissingName         int
	FunctionCallMissingArguments    int
	InvalidInputItemType            int
	MissingOutputSampleIndices      []int
}

// collectResponseAPIInputDiagnostics inspects Response API input items and collects non-sensitive
// diagnostics for malformed tool-history records.
// Parameters: input is the Response API input array to inspect.
// Returns: a diagnostics summary safe for DEBUG logs.
func collectResponseAPIInputDiagnostics(input ResponseAPIInput) responseAPIInputDiagnostics {
	diag := responseAPIInputDiagnostics{}
	for idx, item := range input {
		itemMap, ok := item.(map[string]any)
		if !ok || itemMap == nil {
			if _, isString := item.(string); !isString {
				diag.InvalidInputItemType++
			}
			continue
		}

		typeValue, _ := itemMap["type"].(string)
		switch strings.ToLower(strings.TrimSpace(typeValue)) {
		case "function_call_output":
			if _, ok := itemMap["output"]; !ok {
				diag.FunctionCallOutputMissingOutput++
				if len(diag.MissingOutputSampleIndices) < 5 {
					diag.MissingOutputSampleIndices = append(diag.MissingOutputSampleIndices, idx)
				}
			}
			if callID, _ := itemMap["call_id"].(string); strings.TrimSpace(callID) == "" {
				diag.FunctionCallOutputMissingCallID++
			}
		case "function_call":
			if name, _ := itemMap["name"].(string); strings.TrimSpace(name) == "" {
				diag.FunctionCallMissingName++
			}
			if _, ok := itemMap["arguments"]; !ok {
				diag.FunctionCallMissingArguments++
			}
		}
	}

	return diag
}

func logConvertedRequest(c *gin.Context, metaInfo *meta.Meta, relayMode int, payload any) {
	if c == nil {
		return
	}
	lg := gmw.GetLogger(c)
	if lg == nil {
		return
	}
	fields := []zap.Field{
		zap.Int("relay_mode", relayMode),
	}
	if metaInfo != nil {
		fields = append(fields,
			zap.String("model", metaInfo.ActualModelName),
			zap.Int("channel_id", metaInfo.ChannelId),
		)
	}
	switch req := payload.(type) {
	case *ResponseAPIRequest:
		hasWebSearch, toolTypes := responseAPIToolSummary(req.Tools)
		diag := collectResponseAPIInputDiagnostics(req.Input)
		fields = append(fields,
			zap.String("payload_type", "response_api"),
			zap.Int("input_items", len(req.Input)),
			zap.Int("tool_count", len(req.Tools)),
			zap.Bool("has_web_search_tool", hasWebSearch),
			zap.Int("function_call_output_missing_output", diag.FunctionCallOutputMissingOutput),
			zap.Int("function_call_output_missing_call_id", diag.FunctionCallOutputMissingCallID),
			zap.Int("function_call_missing_name", diag.FunctionCallMissingName),
			zap.Int("function_call_missing_arguments", diag.FunctionCallMissingArguments),
			zap.Int("invalid_response_input_item_type", diag.InvalidInputItemType),
		)
		if len(diag.MissingOutputSampleIndices) > 0 {
			fields = append(fields, zap.Ints("function_call_output_missing_output_indices", diag.MissingOutputSampleIndices))
		}
		if len(toolTypes) > 0 {
			fields = append(fields, zap.Strings("tool_types", toolTypes))
		}
		if req.Reasoning != nil && req.Reasoning.Effort != nil {
			fields = append(fields, zap.String("reasoning_effort", *req.Reasoning.Effort))
		}
		if req.MaxOutputTokens != nil {
			fields = append(fields, zap.Int("max_output_tokens", *req.MaxOutputTokens))
		}
	case *model.GeneralOpenAIRequest:
		hasWebSearch, toolTypes := generalToolSummary(req.Tools)
		fields = append(fields,
			zap.String("payload_type", "chat_completions"),
			zap.Int("message_count", len(req.Messages)),
			zap.Int("tool_count", len(req.Tools)),
			zap.Bool("has_web_search_tool", hasWebSearch),
		)
		if len(toolTypes) > 0 {
			fields = append(fields, zap.Strings("tool_types", toolTypes))
		}
		if req.ReasoningEffort != nil {
			fields = append(fields, zap.String("reasoning_effort", *req.ReasoningEffort))
		}
		if req.MaxCompletionTokens != nil {
			fields = append(fields, zap.Int("max_completion_tokens", *req.MaxCompletionTokens))
		}
	default:
		if payload != nil {
			fields = append(fields, zap.String("payload_type", fmt.Sprintf("%T", payload)))
		}
	}

	lg.Debug("prepared upstream request payload", fields...)
}
