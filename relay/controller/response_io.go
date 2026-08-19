package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// responseCaptureWriter is a gin.ResponseWriter that captures the response body and status code
type responseCaptureWriter struct {
	gin.ResponseWriter
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

// newResponseCaptureWriter creates a new responseCaptureWriter
func newResponseCaptureWriter(w gin.ResponseWriter) *responseCaptureWriter {
	return &responseCaptureWriter{ResponseWriter: w}
}

// Write writes data to the captured body
func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}

// WriteString writes a string to the captured body
func (w *responseCaptureWriter) WriteString(s string) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.WriteString(s)
}

// WriteHeader sets the status code
func (w *responseCaptureWriter) WriteHeader(code int) {
	w.status = code
	w.wroteHeader = true
}

// WriteHeaderNow ensures the header is written
func (w *responseCaptureWriter) WriteHeaderNow() {
	if !w.wroteHeader {
		w.status = w.ResponseWriter.Status()
		if w.status == 0 {
			w.status = http.StatusOK
		}
		w.wroteHeader = true
	}
}

// StatusCode returns the captured status code
func (w *responseCaptureWriter) StatusCode() int {
	if w.status > 0 {
		return w.status
	}
	if code := w.ResponseWriter.Status(); code > 0 {
		return code
	}
	return http.StatusOK
}

// BodyBytes returns the captured body as bytes
func (w *responseCaptureWriter) BodyBytes() []byte {
	return w.body.Bytes()
}

// HeaderWritten returns true if the header has been written
func (w *responseCaptureWriter) HeaderWritten() bool {
	return w.wroteHeader
}

// Written returns true if any data has been written
func (w *responseCaptureWriter) Written() bool {
	if w.wroteHeader || w.body.Len() > 0 {
		return true
	}
	return w.ResponseWriter.Written()
}

// Size returns the size of the captured body
func (w *responseCaptureWriter) Size() int {
	if w.body.Len() > 0 {
		return w.body.Len()
	}
	return w.ResponseWriter.Size()
}

// getResponseAPIRequestBody gets the request body for Response API requests
func getResponseAPIRequestBody(c *gin.Context, meta *metalib.Meta, responseAPIRequest *openai.ResponseAPIRequest, adaptor adaptor.Adaptor) (io.Reader, error) {
	lg := gmw.GetLogger(c)
	// Prefer forwarding the exact user payload to avoid mutating vendor-specific fields
	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get raw Response API request body")
	}

	patched, stats, changed, err := normalizeResponseAPIRawBody(rawBody, responseAPIRequest, meta.ChannelType)
	if err != nil {
		return nil, errors.Wrap(err, "normalize Response API request body")
	}
	if config.DebugEnabled && changed {
		// Avoid logging the payload; only emit shape/count diagnostics.
		lg.Debug("normalized Response API request payload",
			zap.Int("assistant_input_text_fixed", stats.AssistantInputTextFixed),
			zap.Int("non_assistant_output_text_fixed", stats.NonAssistantOutputTextFixed),
			zap.Int("reasoning_summary_fixed", stats.ReasoningSummaryFixed),
			zap.Int("embedded_image_dataurl_redacted", stats.DataURLRedacted),
			zap.Int("embedded_image_dataurl_redacted_bytes", stats.DataURLRedactedBytes),
		)
	}

	return bytes.NewReader(patched), nil
}

// normalizeResponseAPIRawBody normalizes the raw request body for Response API requests
func normalizeResponseAPIRawBody(rawBody []byte, request *openai.ResponseAPIRequest, channelType int) ([]byte, openai.ResponseAPIInputContentNormalizationStats, bool, error) {
	var stats openai.ResponseAPIInputContentNormalizationStats
	if request == nil {
		return rawBody, stats, false, nil
	}

	if len(rawBody) == 0 {
		patched, err := json.Marshal(request)
		if err != nil {
			return rawBody, stats, false, errors.Wrap(err, "marshal response API request")
		}
		return patched, stats, true, nil
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &root); err != nil {
		patched, err2 := json.Marshal(request)
		if err2 != nil {
			return rawBody, stats, false, errors.Wrap(err2, "marshal response API request after unmarshal failure")
		}
		return patched, stats, true, nil
	}

	changed := false

	if request.Model != "" {
		modelBytes, err := json.Marshal(request.Model)
		if err != nil {
			return nil, stats, false, errors.Wrap(err, "marshal mapped model value")
		}
		if existing, ok := root["model"]; !ok || !bytes.Equal(existing, modelBytes) {
			root["model"] = modelBytes
			changed = true
		}
	}

	// Sync previous_response_id from the typed request so the same-provider handle
	// rewrite (ST-021) actually reaches the upstream. In the common case the typed
	// value equals the raw value and this is a no-op; it never invents a selector
	// the client did not send (a nil pointer leaves the raw body untouched).
	if request.PreviousResponseId != nil {
		prevBytes, err := json.Marshal(*request.PreviousResponseId)
		if err != nil {
			return nil, stats, false, errors.Wrap(err, "marshal previous_response_id")
		}
		if existing, ok := root["previous_response_id"]; !ok || !bytes.Equal(existing, prevBytes) {
			root["previous_response_id"] = prevBytes
			changed = true
		}
	}

	if request.ToolChoice == nil {
		if _, ok := root["tool_choice"]; ok {
			delete(root, "tool_choice")
			changed = true
		}
	} else {
		choiceBytes, err := json.Marshal(request.ToolChoice)
		if err != nil {
			return nil, stats, false, errors.Wrap(err, "marshal request tool_choice")
		}
		if existing, ok := root["tool_choice"]; !ok || !bytes.Equal(existing, choiceBytes) {
			root["tool_choice"] = choiceBytes
			changed = true
		}
	}

	if request.Temperature == nil {
		if _, ok := root["temperature"]; ok {
			delete(root, "temperature")
			changed = true
		}
	}

	if request.TopP == nil {
		if _, ok := root["top_p"]; ok {
			delete(root, "top_p")
			changed = true
		}
	}

	if request.Text == nil {
		if _, ok := root["text"]; ok {
			delete(root, "text")
			changed = true
		}
	} else {
		textBytes, err := json.Marshal(request.Text)
		if err != nil {
			return nil, stats, false, errors.Wrap(err, "marshal sanitized response text config")
		}
		if existing, ok := root["text"]; !ok || !bytes.Equal(existing, textBytes) {
			root["text"] = textBytes
			changed = true
		}
	}

	// Normalize reasoning.summary for channels that follow OpenAI's strict validation.
	if request.Reasoning != nil && (channelType == channeltype.OpenAI || channelType == channeltype.Azure) {
		norm := openai.NormalizeResponseReasoningSummaryForModel(request.Model, request.Reasoning)
		if norm.Changed {
			stats.ReasoningSummaryFixed++
			reasoningMap := map[string]any{}
			if rawReasoning, ok := root["reasoning"]; ok && len(rawReasoning) > 0 {
				_ = json.Unmarshal(rawReasoning, &reasoningMap)
				if reasoningMap == nil {
					reasoningMap = map[string]any{}
				}
			}
			if request.Reasoning.Summary == nil {
				delete(reasoningMap, "summary")
			} else {
				reasoningMap["summary"] = *request.Reasoning.Summary
			}
			reasoningBytes, err := json.Marshal(reasoningMap)
			if err != nil {
				return nil, stats, false, errors.Wrap(err, "marshal normalized reasoning config")
			}
			root["reasoning"] = reasoningBytes
			changed = true
		}
	}

	// Backward-compat: normalize historical assistant/user message content item types.
	// Some clients send assistant history with type=input_text, but OpenAI expects output_text (or refusal) for assistant.
	if rawInput, ok := root["input"]; ok && len(rawInput) > 0 {
		parsed := openai.ResponseAPIInput{}
		if err := json.Unmarshal(rawInput, &parsed); err == nil {
			inputStats, inputChanged := openai.NormalizeResponseAPIInputContentTypes(&parsed)
			dataURLStats, dataURLChanged := openai.NormalizeResponseAPIInputEmbeddedImageDataURLs(&parsed)
			if inputChanged || dataURLChanged {
				stats.AssistantInputTextFixed += inputStats.AssistantInputTextFixed
				stats.NonAssistantOutputTextFixed += inputStats.NonAssistantOutputTextFixed
				stats.DataURLRedacted += dataURLStats.DataURLRedacted
				stats.DataURLRedactedBytes += dataURLStats.DataURLRedactedBytes
				request.Input = parsed
				inputBytes, err := json.Marshal(parsed)
				if err != nil {
					return nil, stats, false, errors.Wrap(err, "marshal normalized response input")
				}
				root["input"] = inputBytes
				changed = true
			}
		}
	}

	if len(request.Tools) == 0 {
		if _, ok := root["tools"]; ok {
			delete(root, "tools")
			changed = true
		}
	} else {
		toolsBytes, err := json.Marshal(request.Tools)
		if err != nil {
			return nil, stats, false, errors.Wrap(err, "marshal sanitized response tools")
		}

		// Compare only the typed representation to determine whether sanitization
		// actually changed the tools. If it did not, retain the raw array so custom
		// provider fields (for example Codex apply_patch grammar metadata) survive
		// native Responses passthrough.
		existing, exists := root["tools"]
		typedToolsChanged := true
		if exists {
			var originalTools []openai.ResponseAPITool
			if err := json.Unmarshal(existing, &originalTools); err == nil {
				if originalTypedBytes, err := json.Marshal(originalTools); err == nil {
					typedToolsChanged = !bytes.Equal(originalTypedBytes, toolsBytes)
				}
			}
		}
		if !exists || typedToolsChanged {
			if exists {
				mergedTools, mergeErr := mergeResponseToolsPreservingUnknown(existing, request.Tools)
				if mergeErr != nil {
					return nil, stats, false, errors.Wrap(mergeErr, "merge sanitized response tools")
				}
				toolsBytes = mergedTools
			}
			root["tools"] = toolsBytes
			changed = true
		}
	}

	if !changed {
		merged, _, mergeChanged, err := mergeControlledPassthroughJSON(rawBody, rawBody, false)
		if err != nil {
			return nil, stats, false, errors.Wrap(err, "merge response passthrough fields")
		}
		return merged, stats, mergeChanged, nil
	}

	patched, err := json.Marshal(root)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "marshal patched Response API request")
	}

	merged, _, mergeChanged, err := mergeControlledPassthroughJSON(rawBody, patched, false)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "merge response passthrough fields")
	}

	return merged, stats, changed || mergeChanged, nil
}

// mergeResponseToolsPreservingUnknown overlays sanitized typed tool fields on
// their original raw objects while retaining provider extensions unknown to
// one-api. The returned JSON array follows the sanitized tool order.
func mergeResponseToolsPreservingUnknown(original json.RawMessage, sanitized []openai.ResponseAPITool) ([]byte, error) {
	var rawTools []json.RawMessage
	if err := json.Unmarshal(original, &rawTools); err != nil {
		return nil, errors.Wrap(err, "unmarshal raw response tools")
	}
	var originalTools []openai.ResponseAPITool
	if err := json.Unmarshal(original, &originalTools); err != nil {
		return nil, errors.Wrap(err, "unmarshal typed response tools")
	}

	used := make([]bool, len(originalTools))
	merged := make([]json.RawMessage, 0, len(sanitized))
	for _, sanitizedTool := range sanitized {
		match := -1
		identity := responseToolIdentity(sanitizedTool)
		for idx := range originalTools {
			if used[idx] || responseToolIdentity(originalTools[idx]) != identity {
				continue
			}
			match = idx
			break
		}

		sanitizedBytes, err := json.Marshal(sanitizedTool)
		if err != nil {
			return nil, errors.Wrap(err, "marshal sanitized response tool")
		}
		if match < 0 || match >= len(rawTools) {
			merged = append(merged, sanitizedBytes)
			continue
		}
		used[match] = true

		originalTypedBytes, err := json.Marshal(originalTools[match])
		if err == nil && bytes.Equal(originalTypedBytes, sanitizedBytes) {
			merged = append(merged, rawTools[match])
			continue
		}

		var rawObject map[string]json.RawMessage
		if err := json.Unmarshal(rawTools[match], &rawObject); err != nil {
			merged = append(merged, sanitizedBytes)
			continue
		}
		var sanitizedObject map[string]json.RawMessage
		if err := json.Unmarshal(sanitizedBytes, &sanitizedObject); err != nil {
			return nil, errors.Wrap(err, "unmarshal sanitized response tool")
		}
		for _, key := range responseToolKnownJSONKeys {
			delete(rawObject, key)
		}
		for key, value := range sanitizedObject {
			rawObject[key] = value
		}
		mergedBytes, err := json.Marshal(rawObject)
		if err != nil {
			return nil, errors.Wrap(err, "marshal merged response tool")
		}
		merged = append(merged, mergedBytes)
	}

	result, err := json.Marshal(merged)
	if err != nil {
		return nil, errors.Wrap(err, "marshal merged response tool array")
	}
	return result, nil
}

var responseToolKnownJSONKeys = []string{
	"type", "name", "description", "parameters", "function",
	"search_context_size", "filters", "user_location",
	"server_label", "server_url", "require_approval", "allowed_tools", "headers",
}

// responseToolIdentity returns a stable matching key for correlating a typed
// sanitized tool with its original raw object.
func responseToolIdentity(tool openai.ResponseAPITool) string {
	name := tool.Name
	if tool.Function != nil && tool.Function.Name != "" {
		name = tool.Function.Name
	}
	return strings.ToLower(strings.TrimSpace(tool.Type)) + "\x00" + name + "\x00" + tool.ServerLabel
}
