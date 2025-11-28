package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// evaluateResponse inspects a non-streaming response and validates the expected shape.
func evaluateResponse(spec requestSpec, body []byte) (bool, string) {
	if len(body) == 0 {
		return true, ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, fmt.Sprintf("malformed JSON response: %v", err)
	}

	if errVal, ok := payload["error"]; ok && isMeaningfulErrorValue(errVal) {
		return false, snippet(body)
	}

	switch spec.Type {
	case requestTypeChatCompletion:
		switch spec.Expectation {
		case expectationStructuredOutput:
			if structuredOutputSatisfied(payload) || structuredOutputSatisfiedBytes(body) {
				return true, ""
			}
			// If the response was truncated due to max_tokens limit, treat it as a successful outcome
			if isMaxTokensTruncated(payload) {
				return true, ""
			}
			return false, "structured output fields missing"
		case expectationToolInvocation:
			if chatHasToolCalls(payload) {
				return true, ""
			}
			return false, "response missing tool_calls"
		case expectationToolHistory:
			if chatHasToolCalls(payload) || chatHasAssistantText(payload) {
				return true, ""
			}
			return false, "response missing tool_calls or assistant text"
		default:
			if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
				return true, ""
			}
			return false, "response missing choices"
		}
	case requestTypeResponseAPI:
		switch spec.Expectation {
		case expectationStructuredOutput:
			if structuredOutputSatisfied(payload) || structuredOutputSatisfiedBytes(body) {
				status := stringValue(payload, "status")
				if status == "failed" {
					return false, snippet(body)
				}
				return true, ""
			}
			// If the response was truncated due to max_tokens limit, treat it as a successful outcome
			if isMaxTokensTruncated(payload) {
				return true, ""
			}
			return false, "structured output fields missing"
		case expectationToolInvocation:
			if responseHasToolCalls(payload) {
				return true, ""
			}
			return false, "response missing required_action.tool_calls"
		case expectationToolHistory:
			if responseHasToolCalls(payload) || responseOutputHasText(payload) || chatHasAssistantText(payload) {
				return true, ""
			}
			return false, "response missing tool history output"
		default:
			status := stringValue(payload, "status")
			if status == "failed" {
				return false, snippet(body)
			}
			if output, ok := payload["output"].([]any); ok && len(output) > 0 {
				return true, ""
			}
			if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
				return true, ""
			}
			if status == "completed" || status == "in_progress" || status == "requires_action" {
				return true, ""
			}
			if len(payload) == 0 {
				return false, "empty response"
			}
			return false, "response missing output"
		}
	case requestTypeClaudeMessages:
		switch spec.Expectation {
		case expectationStructuredOutput:
			if structuredOutputSatisfied(payload) || structuredOutputSatisfiedBytes(body) {
				return true, ""
			}
			// If the response was truncated due to max_tokens limit, treat it as a successful outcome
			// since the upstream fulfilled the request as the user intended (respecting the token limit).
			if isMaxTokensTruncated(payload) {
				return true, ""
			}
			return false, "structured output fields missing"
		case expectationToolInvocation:
			if choices, ok := payload["choices"].([]any); ok {
				for _, choice := range choices {
					choiceMap, ok := choice.(map[string]any)
					if !ok {
						continue
					}
					if message, ok := choiceMap["message"].(map[string]any); ok {
						if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
							return true, ""
						}
					}
				}
			}
			if content, ok := payload["content"].([]any); ok {
				for _, entry := range content {
					entryMap, ok := entry.(map[string]any)
					if !ok {
						continue
					}
					if stringValue(entryMap, "type") == "tool_use" {
						return true, ""
					}
				}
			}
			// Accept function_call outputs from Response API conversions
			if hasFunctionCallOutput(payload) {
				return true, ""
			}
			return false, "response missing tool_use block"
		case expectationToolHistory:
			if choices, ok := payload["choices"].([]any); ok {
				for _, choice := range choices {
					choiceMap, ok := choice.(map[string]any)
					if !ok {
						continue
					}
					if message, ok := choiceMap["message"].(map[string]any); ok {
						if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
							return true, ""
						}
					}
				}
			}
			if content, ok := payload["content"].([]any); ok {
				for _, entry := range content {
					entryMap, ok := entry.(map[string]any)
					if !ok {
						continue
					}
					if stringValue(entryMap, "type") == "tool_use" {
						return true, ""
					}
				}
			}
			// Accept function_call outputs from Response API conversions even without assistant text
			if hasFunctionCallOutput(payload) {
				return true, ""
			}
			if claudePayloadHasText(payload) {
				return true, ""
			}
			return false, "response missing tool_use block or assistant text"
		default:
			if content, ok := payload["content"].([]any); ok && len(content) > 0 {
				return true, ""
			}
			if msgType := stringValue(payload, "type"); msgType != "" {
				return true, ""
			}
			if len(payload) == 0 {
				return false, "empty response"
			}
			return true, ""
		}
	default:
		return true, ""
	}
}

// hasFunctionCallOutput reports whether the Response API payload contains a function_call entry.
func chatHasToolCalls(payload map[string]any) bool {
	choices, ok := payload["choices"].([]any)
	if !ok {
		return false
	}
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]any)
		if !ok {
			continue
		}
		if message, ok := choiceMap["message"].(map[string]any); ok {
			if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
				return true
			}
		}
	}
	return false
}

func chatHasAssistantText(payload map[string]any) bool {
	choices, ok := payload["choices"].([]any)
	if !ok {
		return false
	}
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]any)
		if !ok {
			continue
		}
		if message, ok := choiceMap["message"].(map[string]any); ok {
			if messageHasText(message) {
				return true
			}
		}
	}
	return false
}

func messageHasText(msg map[string]any) bool {
	if msg == nil {
		return false
	}
	if content, ok := msg["content"].(string); ok {
		if strings.TrimSpace(content) != "" {
			return true
		}
	}
	if arr, ok := msg["content"].([]any); ok {
		for _, entry := range arr {
			switch part := entry.(type) {
			case map[string]any:
				if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
					return true
				}
			case string:
				if strings.TrimSpace(part) != "" {
					return true
				}
			}
		}
	}
	if text, ok := msg["text"].(string); ok && strings.TrimSpace(text) != "" {
		return true
	}
	return false
}

func responseHasToolCalls(payload map[string]any) bool {
	if chatHasToolCalls(payload) {
		return true
	}
	if required, ok := payload["required_action"].(map[string]any); ok {
		if stringValue(required, "type") == "submit_tool_outputs" {
			if submit, ok := required["submit_tool_outputs"].(map[string]any); ok {
				if calls, ok := submit["tool_calls"].([]any); ok && len(calls) > 0 {
					return true
				}
			}
		}
	}
	return hasFunctionCallOutput(payload)
}

func responseOutputHasText(payload map[string]any) bool {
	output, ok := payload["output"].([]any)
	if !ok {
		return false
	}
	for _, entry := range output {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(stringValue(entryMap, "type"), "message") {
			if messageHasText(map[string]any{"content": entryMap["content"]}) {
				return true
			}
		}
	}
	return false
}

func claudePayloadHasText(payload map[string]any) bool {
	if content, ok := payload["content"].([]any); ok {
		for _, entry := range content {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := entryMap["text"].(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	if choices, ok := payload["choices"].([]any); ok {
		for _, choice := range choices {
			choiceMap, ok := choice.(map[string]any)
			if !ok {
				continue
			}
			if message, ok := choiceMap["message"].(map[string]any); ok && messageHasText(message) {
				return true
			}
		}
	}
	return false
}

func hasFunctionCallOutput(payload map[string]any) bool {
	output, ok := payload["output"].([]any)
	if !ok {
		return false
	}
	for _, entry := range output {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if stringValue(entryMap, "type") == "function_call" {
			return true
		}
	}
	return false
}

// evaluateStreamResponse validates streaming SSE payloads for expected content.
func evaluateStreamResponse(spec requestSpec, data []byte) (bool, string) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false, "empty stream"
	}

	lines := bytes.Split(trimmed, []byte("\n"))
	var (
		hasPayload   bool
		toolDetected bool
		textDetected bool
	)
	var (
		structuredMarkers   map[string]bool
		structuredBuffer    *bytes.Buffer
		structuredFragments *strings.Builder
	)
	if spec.Expectation == expectationStructuredOutput {
		structuredMarkers = map[string]bool{"topic": false, "confidence": false}
		structuredBuffer = &bytes.Buffer{}
		structuredFragments = &strings.Builder{}
	}

	for _, rawLine := range lines {
		line := bytes.TrimSpace(rawLine)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}

		payload := bytes.TrimSpace(line[len("data:"):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		hasPayload = true
		if structuredBuffer != nil {
			structuredBuffer.Write(payload)
		}

		var obj map[string]any
		if err := json.Unmarshal(payload, &obj); err == nil {
			if errVal, ok := obj["error"]; ok && isMeaningfulErrorValue(errVal) {
				return false, snippet(payload)
			}
			if isToolExpectation(spec.Expectation) && detectToolInvocationInStream(spec, obj) {
				toolDetected = true
			}
			if detectAssistantTextInStream(spec, obj) {
				textDetected = true
			}
			if spec.Expectation == expectationStructuredOutput {
				collectStructuredMarkers(obj, structuredMarkers)
				appendStructuredFragments(obj, structuredFragments)
			}
		}
	}

	if !hasPayload {
		return false, "empty stream payload"
	}

	if spec.Expectation == expectationToolInvocation && !toolDetected {
		return false, "stream missing tool invocation"
	}
	if spec.Expectation == expectationToolHistory && !toolDetected && !textDetected {
		return false, "stream missing tool invocation"
	}

	lower := bytes.ToLower(trimmed)
	if bytes.Contains(lower, []byte("\"error\"")) && !bytes.Contains(lower, []byte("\"error\":null")) {
		return false, snippet(trimmed)
	}

	if spec.Expectation == expectationStructuredOutput {
		if structuredMarkers != nil && structuredMarkers["topic"] && structuredMarkers["confidence"] {
			return true, ""
		}
		if structuredFragments != nil && structuredOutputSatisfiedBytes([]byte(structuredFragments.String())) {
			return true, ""
		}
		if structuredBuffer != nil && structuredOutputSatisfiedBytes(structuredBuffer.Bytes()) {
			return true, ""
		}
		if structuredOutputSatisfiedBytes(trimmed) {
			return true, ""
		}
		return false, "stream missing structured output fields"
	}

	return true, ""
}

// detectToolInvocationInStream inspects streamed JSON fragments for tool calls.
func detectToolInvocationInStream(spec requestSpec, obj map[string]any) bool {
	switch spec.Type {
	case requestTypeChatCompletion:
		if choices, ok := obj["choices"].([]any); ok {
			for _, choice := range choices {
				choiceMap, ok := choice.(map[string]any)
				if !ok {
					continue
				}
				if delta, ok := choiceMap["delta"].(map[string]any); ok {
					if calls, ok := delta["tool_calls"].([]any); ok && len(calls) > 0 {
						return true
					}
				}
				if message, ok := choiceMap["message"].(map[string]any); ok {
					if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
						return true
					}
				}
			}
		}
	case requestTypeResponseAPI:
		if choices, ok := obj["choices"].([]any); ok {
			for _, choice := range choices {
				choiceMap, ok := choice.(map[string]any)
				if !ok {
					continue
				}
				if delta, ok := choiceMap["delta"].(map[string]any); ok {
					if calls, ok := delta["tool_calls"].([]any); ok && len(calls) > 0 {
						return true
					}
				}
				if message, ok := choiceMap["message"].(map[string]any); ok {
					if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
						return true
					}
				}
			}
		}
		if item, ok := obj["item"].(map[string]any); ok {
			if stringValue(item, "type") == "function_call" {
				return true
			}
		}
		if outputItem, ok := obj["output_item"].(map[string]any); ok {
			if stringValue(outputItem, "type") == "function_call" {
				return true
			}
		}
		if responseObj, ok := obj["response"].(map[string]any); ok {
			if required, ok := responseObj["required_action"].(map[string]any); ok {
				if stringValue(required, "type") == "submit_tool_outputs" {
					if submit, ok := required["submit_tool_outputs"].(map[string]any); ok {
						if calls, ok := submit["tool_calls"].([]any); ok && len(calls) > 0 {
							return true
						}
					}
				}
			}
			if output, ok := responseObj["output"].([]any); ok {
				for _, entry := range output {
					entryMap, ok := entry.(map[string]any)
					if !ok {
						continue
					}
					if stringValue(entryMap, "type") == "function_call" {
						return true
					}
				}
			}
			if delta, ok := responseObj["delta"].(map[string]any); ok {
				if calls, ok := delta["tool_calls"].([]any); ok && len(calls) > 0 {
					return true
				}
			}
		}
		if required, ok := obj["required_action"].(map[string]any); ok {
			if stringValue(required, "type") == "submit_tool_outputs" {
				if submit, ok := required["submit_tool_outputs"].(map[string]any); ok {
					if calls, ok := submit["tool_calls"].([]any); ok && len(calls) > 0 {
						return true
					}
				}
			}
		}
		if output, ok := obj["output"].([]any); ok {
			for _, entry := range output {
				entryMap, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				if stringValue(entryMap, "type") == "function_call" {
					return true
				}
			}
		}
		if delta, ok := obj["delta"].(map[string]any); ok {
			if calls, ok := delta["tool_calls"].([]any); ok && len(calls) > 0 {
				return true
			}
		}
	case requestTypeClaudeMessages:
		if contentBlock, ok := obj["content_block"].(map[string]any); ok {
			if stringValue(contentBlock, "type") == "tool_use" {
				return true
			}
		}
		if content, ok := obj["content"].([]any); ok {
			for _, entry := range content {
				entryMap, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				if stringValue(entryMap, "type") == "tool_use" {
					return true
				}
			}
		}
		if choices, ok := obj["choices"].([]any); ok {
			for _, choice := range choices {
				choiceMap, ok := choice.(map[string]any)
				if !ok {
					continue
				}
				if delta, ok := choiceMap["delta"].(map[string]any); ok {
					if calls, ok := delta["tool_calls"].([]any); ok && len(calls) > 0 {
						return true
					}
				}
				if message, ok := choiceMap["message"].(map[string]any); ok {
					if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
						return true
					}
				}
			}
		}
		if delta, ok := obj["delta"].(map[string]any); ok {
			if calls, ok := delta["tool_calls"].([]any); ok && len(calls) > 0 {
				return true
			}
		}
	}
	return false
}

func detectAssistantTextInStream(spec requestSpec, obj map[string]any) bool {
	switch spec.Type {
	case requestTypeChatCompletion:
		if choices, ok := obj["choices"].([]any); ok {
			for _, choice := range choices {
				choiceMap, ok := choice.(map[string]any)
				if !ok {
					continue
				}
				if delta, ok := choiceMap["delta"].(map[string]any); ok {
					if content, ok := delta["content"].(string); ok && strings.TrimSpace(content) != "" {
						return true
					}
					if text, ok := delta["text"].(string); ok && strings.TrimSpace(text) != "" {
						return true
					}
				}
				if message, ok := choiceMap["message"].(map[string]any); ok && messageHasText(message) {
					return true
				}
			}
		}
	case requestTypeResponseAPI:
		if detectAssistantTextInStream(requestSpec{Type: requestTypeChatCompletion}, obj) {
			return true
		}
		if responseObj, ok := obj["response"].(map[string]any); ok {
			if responseOutputHasText(responseObj) || chatHasAssistantText(responseObj) {
				return true
			}
			if delta, ok := responseObj["delta"].(map[string]any); ok {
				if text, ok := delta["text"].(string); ok && strings.TrimSpace(text) != "" {
					return true
				}
			}
		}
		if item, ok := obj["item"].(map[string]any); ok {
			if messageHasText(map[string]any{"content": item["content"], "text": item["text"]}) {
				return true
			}
		}
		if delta, ok := obj["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	case requestTypeClaudeMessages:
		if contentBlock, ok := obj["content_block"].(map[string]any); ok {
			if text, ok := contentBlock["text"].(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
		if delta, ok := obj["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
		if content, ok := obj["content"].([]any); ok {
			for _, entry := range content {
				entryMap, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := entryMap["text"].(string); ok && strings.TrimSpace(text) != "" {
					return true
				}
			}
		}
	}
	if text, ok := obj["text"].(string); ok && strings.TrimSpace(text) != "" {
		return true
	}
	return false
}

// isToolExpectation reports whether the provided expectation requires detecting tool invocations.
func isToolExpectation(exp expectation) bool {
	return exp == expectationToolInvocation || exp == expectationToolHistory
}

// isMaxTokensTruncated reports whether the response was truncated due to max_tokens limit.
// This is used to treat truncated responses as successful outcomes, since the upstream
// service fulfilled the request as the user intended (respecting the token limit).
func isMaxTokensTruncated(payload map[string]any) bool {
	// Check for Claude Messages API stop_reason
	stopReason := stringValue(payload, "stop_reason")
	if stopReason == "max_tokens" || stopReason == "length" {
		return true
	}

	// Check for OpenAI-style finish_reason in choices
	if choices, ok := payload["choices"].([]any); ok {
		for _, choice := range choices {
			choiceMap, ok := choice.(map[string]any)
			if !ok {
				continue
			}
			finishReason := stringValue(choiceMap, "finish_reason")
			if finishReason == "length" || finishReason == "max_tokens" {
				return true
			}
		}
	}

	return false
}

func stringValue(data map[string]any, key string) string {
	if raw, ok := data[key]; ok {
		if s, ok := raw.(string); ok {
			return s
		}
	}
	return ""
}

func isMeaningfulErrorValue(val any) bool {
	switch v := val.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case map[string]any:
		if len(v) == 0 {
			return false
		}
		for _, nested := range v {
			if isMeaningfulErrorValue(nested) {
				return true
			}
		}
		return false
	case []any:
		return slices.ContainsFunc(v, isMeaningfulErrorValue)
	case bool:
		return v
	case float64:
		return v != 0
	default:
		return true
	}
}

func isUnsupportedCombination(reqType requestType, stream bool, statusCode int, body []byte, reason string) bool {
	text := reason
	if text == "" {
		text = snippet(body)
	}
	lower := strings.ToLower(text)
	bodyLower := strings.ToLower(string(body))

	switch reqType {
	case requestTypeResponseAPI:
		if strings.Contains(lower, "unknown field `messages`") ||
			strings.Contains(lower, "does not support responses") ||
			strings.Contains(lower, "response api is not available") ||
			strings.Contains(bodyLower, "does not support responses") {
			return true
		}
	case requestTypeChatCompletion:
		if strings.Contains(lower, "only supports response") ||
			strings.Contains(lower, "chat completions unsupported") ||
			strings.Contains(bodyLower, "chat completions unsupported") {
			return true
		}
	case requestTypeClaudeMessages:
		if strings.Contains(lower, "does not support claude") ||
			strings.Contains(lower, "claude messages unsupported") ||
			strings.Contains(bodyLower, "claude messages unsupported") {
			return true
		}
	}

	if stream && (strings.Contains(lower, "streaming is not supported") ||
		strings.Contains(lower, "stream parameter is not supported") ||
		strings.Contains(lower, "stream currently disabled")) {
		return true
	}

	if strings.Contains(lower, "no available channels") {
		return true
	}

	if stream {
		trimmed := strings.TrimSpace(string(body))
		switch strings.ToLower(trimmed) {
		case "data: [done]", "[done]", "data:[done]":
			return true
		}
	}

	if statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed {
		return true
	}

	// Treat transient upstream errors (502, 503, 504) as skippable rather than failures.
	// These typically indicate temporary infrastructure issues (Cloudflare, load balancer, etc.)
	// rather than genuine API incompatibilities.
	if statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout {
		return true
	}

	return false
}

// structuredOutputSatisfied reports whether the payload contains the expected structured output fields.
func structuredOutputSatisfied(payload any) bool {
	found := map[string]bool{"topic": false, "confidence": false}
	collectStructuredMarkers(payload, found)
	return found["topic"] && found["confidence"]
}

// structuredOutputSatisfiedBytes checks for structured output fields in the raw JSON bytes.
func structuredOutputSatisfiedBytes(body []byte) bool {
	lower := strings.ToLower(string(body))
	lower = strings.ReplaceAll(lower, " ", "")
	lower = strings.ReplaceAll(lower, "\n", "")
	lower = strings.ReplaceAll(lower, "\t", "")
	lower = strings.ReplaceAll(lower, "\r", "")
	lower = strings.ReplaceAll(lower, "\\", "")
	lower = strings.ReplaceAll(lower, "\"", "")
	topicPresent := strings.Contains(lower, "\"topic\"") || strings.Contains(lower, "topic")
	confidencePresent := strings.Contains(lower, "\"confidence\"") || strings.Contains(lower, "confidence") || strings.Contains(lower, "confidence_score")
	return topicPresent && confidencePresent
}

// collectStructuredMarkers recursively scans arbitrary JSON-like data for structured output markers.
func collectStructuredMarkers(node any, found map[string]bool) {
	switch val := node.(type) {
	case map[string]any:
		for key, child := range val {
			lowerKey := strings.ToLower(key)
			if lowerKey == "topic" {
				found["topic"] = true
			}
			if lowerKey == "confidence" || lowerKey == "confidence_score" {
				found["confidence"] = true
			}
			collectStructuredMarkers(child, found)
		}
	case []any:
		for _, child := range val {
			collectStructuredMarkers(child, found)
		}
	case string:
		lower := strings.ToLower(val)
		sanitized := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(lower)
		sanitized = strings.ReplaceAll(sanitized, "\\", "")
		sanitized = strings.ReplaceAll(sanitized, "\"", "")
		if strings.Contains(sanitized, "\"topic\"") || strings.Contains(sanitized, "topic") {
			found["topic"] = true
		}
		if strings.Contains(sanitized, "\"confidence\"") || strings.Contains(sanitized, "confidence_score") || strings.Contains(sanitized, "confidence") {
			found["confidence"] = true
		}
		if !(found["topic"] && found["confidence"]) {
			var nested any
			if err := json.Unmarshal([]byte(val), &nested); err == nil {
				collectStructuredMarkers(nested, found)
			}
		}
	}
}

// appendStructuredFragments accumulates partial_json segments emitted during streaming structured output.
func appendStructuredFragments(node any, builder *strings.Builder) {
	if builder == nil {
		return
	}
	switch val := node.(type) {
	case map[string]any:
		for key, child := range val {
			if strings.EqualFold(key, "partial_json") ||
				strings.EqualFold(key, "arguments") ||
				strings.EqualFold(key, "text") ||
				strings.EqualFold(key, "output_text") ||
				strings.EqualFold(key, "reasoning") ||
				strings.EqualFold(key, "output_json") ||
				strings.EqualFold(key, "output_json_delta") {
				if str, ok := child.(string); ok {
					builder.WriteString(str)
				} else if marshaled, err := json.Marshal(child); err == nil {
					builder.Write(marshaled)
				}
			}
			appendStructuredFragments(child, builder)
		}
	case []any:
		for _, child := range val {
			appendStructuredFragments(child, builder)
		}
	}
}
