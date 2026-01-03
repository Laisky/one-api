package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	metalib "github.com/songquanpeng/one-api/relay/meta"
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

	patched, stats, changed, err := normalizeResponseAPIRawBody(rawBody, responseAPIRequest)
	if err != nil {
		return nil, errors.Wrap(err, "normalize Response API request body")
	}
	if config.DebugEnabled && changed {
		// Avoid logging the payload; only emit shape/count diagnostics.
		lg.Debug("normalized Response API request payload",
			zap.Int("assistant_input_text_fixed", stats.AssistantInputTextFixed),
			zap.Int("non_assistant_output_text_fixed", stats.NonAssistantOutputTextFixed),
		)
	}

	return bytes.NewReader(patched), nil
}

// normalizeResponseAPIRawBody normalizes the raw request body for Response API requests
func normalizeResponseAPIRawBody(rawBody []byte, request *openai.ResponseAPIRequest) ([]byte, openai.ResponseAPIInputContentNormalizationStats, bool, error) {
	var stats openai.ResponseAPIInputContentNormalizationStats
	if request == nil {
		return rawBody, stats, false, nil
	}

	if len(rawBody) == 0 {
		patched, err := json.Marshal(request)
		return patched, stats, err == nil, err
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &root); err != nil {
		patched, err2 := json.Marshal(request)
		return patched, stats, err2 == nil, err2
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

	// Backward-compat: normalize historical assistant/user message content item types.
	// Some clients send assistant history with type=input_text, but OpenAI expects output_text (or refusal) for assistant.
	if rawInput, ok := root["input"]; ok && len(rawInput) > 0 {
		parsed := openai.ResponseAPIInput{}
		if err := json.Unmarshal(rawInput, &parsed); err == nil {
			inputStats, inputChanged := openai.NormalizeResponseAPIInputContentTypes(&parsed)
			if inputChanged {
				stats = inputStats
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
		if existing, ok := root["tools"]; !ok || !bytes.Equal(existing, toolsBytes) {
			root["tools"] = toolsBytes
			changed = true
		}
	}

	if !changed {
		return rawBody, stats, false, nil
	}

	patched, err := json.Marshal(root)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "marshal patched Response API request")
	}
	return patched, stats, true, nil
}
