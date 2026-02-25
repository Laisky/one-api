package openai

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/tracing"
	dbmodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	rmeta "github.com/songquanpeng/one-api/relay/meta"
)

const wsRequestPreviewLimit = 4096

// responseAPIWSErrorPayload represents the documented WebSocket error event payload.
type responseAPIWSErrorPayload struct {
	Type   string                 `json:"type"`
	Status int                    `json:"status"`
	Error  map[string]any         `json:"error"`
	Raw    map[string]interface{} `json:"-"`
}

const wsErrorCodeConnectionLimitReached = "websocket_connection_limit_reached"

// doResponseAPIRequestViaWebSocket attempts to execute an OpenAI Responses request via
// WebSocket transport and synthesizes an HTTP-like response object for the existing relay pipeline.
//
// Parameters:
//   - c: Gin context for request-scoped logging/tracing.
//   - requestAdaptor: adaptor used for URL and header setup consistency.
//   - metaInfo: relay metadata containing upstream channel and auth settings.
//   - requestBody: request JSON payload reader.
//
// Returns:
//   - *http.Response: synthesized response body (SSE for stream=true; JSON for non-stream).
//   - bool: true when WebSocket path handled the request, false when caller should fallback.
//   - error: transport error during WS setup/execution.
func doResponseAPIRequestViaWebSocket(
	c *gin.Context,
	requestAdaptor adaptor.Adaptor,
	metaInfo *rmeta.Meta,
	requestBody io.Reader,
) (*http.Response, bool, error) {
	lg := gmw.GetLogger(c)
	if metaInfo == nil || requestBody == nil {
		lg.Debug("openai response api websocket not applicable",
			zap.String("reason", "nil_meta_or_body"),
		)
		return nil, false, nil
	}

	payload, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, true, errors.Wrap(err, "read response api request body for websocket")
	}
	if len(payload) == 0 {
		lg.Debug("openai response api websocket not applicable",
			zap.String("reason", "empty_request_body"),
		)
		return nil, false, nil
	}

	preview := payload
	truncated := false
	if len(preview) > wsRequestPreviewLimit {
		preview = preview[:wsRequestPreviewLimit]
		truncated = true
	}

	var requestMap map[string]any
	if err := json.Unmarshal(payload, &requestMap); err != nil {
		lg.Debug("openai response api websocket not applicable",
			zap.String("reason", "request_not_json_object"),
			zap.Error(err),
		)
		return nil, false, nil
	}

	streamingRequested, _ := requestMap["stream"].(bool)
	backgroundRequested, _ := requestMap["background"].(bool)
	if backgroundRequested {
		lg.Debug("openai response api websocket skipped",
			zap.String("reason", "background_true_requires_http"),
			zap.String("model", metaInfo.ActualModelName),
		)
		return nil, false, nil
	}

	fullRequestURL, err := requestAdaptor.GetRequestURL(metaInfo)
	if err != nil {
		return nil, true, errors.Wrap(err, "resolve response api websocket request url")
	}

	wsURL, err := toResponseAPIWebSocketURL(fullRequestURL)
	if err != nil {
		return nil, true, errors.Wrap(err, "build response api websocket url")
	}

	dialHeader, err := buildResponseAPIWebSocketHeader(c, requestAdaptor, metaInfo, fullRequestURL)
	if err != nil {
		return nil, true, errors.Wrap(err, "setup response api websocket headers")
	}

	lg.Debug("openai response api websocket connecting",
		zap.String("ws_url", wsURL),
		zap.Bool("stream", streamingRequested),
		zap.String("model", metaInfo.ActualModelName),
		zap.Int("body_bytes", len(payload)),
		zap.Bool("body_truncated", truncated),
		zap.ByteString("body_preview", preview),
	)

	requestMap["type"] = "response.create"
	delete(requestMap, "stream")
	delete(requestMap, "background")

	eventBody, err := json.Marshal(requestMap)
	if err != nil {
		return nil, true, errors.Wrap(err, "marshal response.create websocket event")
	}

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 15 * time.Second,
	}

	if dbmodel.DB != nil {
		tracing.RecordTraceTimestamp(c, dbmodel.TimestampRequestForwarded)
	}
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)

	upstreamConn, _, err := dialer.Dial(wsURL, dialHeader)
	if err != nil {
		return nil, true, errors.Wrap(err, "dial openai response websocket")
	}

	if err := upstreamConn.WriteMessage(websocket.TextMessage, eventBody); err != nil {
		_ = upstreamConn.Close()
		return nil, true, errors.Wrap(err, "send response.create websocket event")
	}

	lg.Debug("openai response api websocket event sent",
		zap.String("event_type", "response.create"),
		zap.Bool("stream", streamingRequested),
		zap.String("model", metaInfo.ActualModelName),
	)

	firstMessage, firstErr := readNextWebSocketTextMessage(upstreamConn)
	if firstErr != nil {
		if closeCode, closeReason, normalClosure := extractNormalWebSocketClose(firstErr); normalClosure {
			lg.Debug("openai response api websocket closed before first event; fallback to http",
				zap.String("model", metaInfo.ActualModelName),
				zap.Int("close_code", closeCode),
				zap.String("close_reason", closeReason),
			)
			_ = upstreamConn.Close()
			return nil, false, nil
		}

		if closeCode, closeReason, hasClose := extractWebSocketCloseDetails(firstErr); hasClose {
			lg.Debug("openai response api websocket read failed on first event",
				zap.String("model", metaInfo.ActualModelName),
				zap.Int("close_code", closeCode),
				zap.String("close_reason", closeReason),
			)
		}

		_ = upstreamConn.Close()
		return nil, true, errors.Wrap(firstErr, "read first websocket response event")
	}
	if dbmodel.DB != nil {
		tracing.RecordTraceTimestamp(c, dbmodel.TimestampFirstUpstreamResponse)
	}

	if errResp, ok := tryBuildWebSocketErrorResponse(firstMessage); ok {
		if shouldFallbackToHTTPForWebSocketError(firstMessage) {
			lg.Debug("openai response api websocket upstream requested reconnection; fallback to http",
				zap.String("model", metaInfo.ActualModelName),
				zap.String("fallback_reason", wsErrorCodeConnectionLimitReached),
			)
			_ = upstreamConn.Close()
			return nil, false, nil
		}

		lg.Warn("openai response api websocket returned upstream error event",
			zap.Int("status_code", errResp.StatusCode),
			zap.String("model", metaInfo.ActualModelName),
			zap.String("error_code", readWebSocketErrorCode(firstMessage)),
			zap.String("error_type", readWebSocketErrorType(firstMessage)),
		)
		_ = upstreamConn.Close()
		return errResp, true, nil
	}

	if streamingRequested {
		lg.Debug("openai response api websocket stream bridge enabled",
			zap.String("model", metaInfo.ActualModelName),
		)
		streamResp := buildStreamingWebSocketHTTPResponse(upstreamConn, firstMessage)
		return streamResp, true, nil
	}

	lg.Debug("openai response api websocket non-stream aggregation enabled",
		zap.String("model", metaInfo.ActualModelName),
	)
	nonStreamResp, err := buildNonStreamingWebSocketHTTPResponse(upstreamConn, firstMessage)
	if err != nil {
		return nil, true, errors.Wrap(err, "collect websocket response for non-stream request")
	}
	return nonStreamResp, true, nil
}

// toResponseAPIWebSocketURL converts a regular HTTP(S) /v1/responses URL to its
// WebSocket endpoint URL.
//
// Parameters:
//   - fullRequestURL: upstream HTTP URL generated by the adaptor.
//
// Returns:
//   - string: ws/wss URL pointing to /v1/responses.
//   - error: when URL parsing fails.
func toResponseAPIWebSocketURL(fullRequestURL string) (string, error) {
	parsed, err := url.Parse(fullRequestURL)
	if err != nil {
		return "", errors.Wrap(err, "parse full request url")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https", "":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "wss"
	}
	return parsed.String(), nil
}

// buildResponseAPIWebSocketHeader builds upstream websocket headers.
//
// Parameters:
//   - c: Gin context that carries caller headers.
//   - metaInfo: relay metadata with API key.
//
// Returns:
//   - http.Header: dial headers for upstream websocket handshake.
func buildResponseAPIWebSocketHeader(
	c *gin.Context,
	requestAdaptor adaptor.Adaptor,
	metaInfo *rmeta.Meta,
	fullRequestURL string,
) (http.Header, error) {
	request, err := http.NewRequest(http.MethodPost, fullRequestURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "create placeholder request for websocket headers")
	}

	if err := requestAdaptor.SetupRequestHeader(c, request, metaInfo); err != nil {
		return nil, errors.Wrap(err, "setup adaptor request headers")
	}

	header := http.Header{}
	for key, values := range request.Header {
		for _, value := range values {
			header.Add(key, value)
		}
	}

	return header, nil
}

// readNextWebSocketTextMessage reads frames until the next text message is available.
//
// Parameters:
//   - conn: connected upstream websocket.
//
// Returns:
//   - []byte: text payload.
//   - error: read/connection errors.
func readNextWebSocketTextMessage(conn *websocket.Conn) ([]byte, error) {
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if messageType == websocket.TextMessage {
			return payload, nil
		}
	}
}

// extractWebSocketCloseDetails extracts close code and reason from websocket errors,
// including wrapped errors.
//
// Parameters:
//   - err: websocket read/write error.
//
// Returns:
//   - int: close code when available.
//   - string: close reason text.
//   - bool: true when the error chain contains websocket.CloseError.
func extractWebSocketCloseDetails(err error) (int, string, bool) {
	if err == nil {
		return 0, "", false
	}

	var closeErr *websocket.CloseError
	if !stderrors.As(err, &closeErr) {
		return 0, "", false
	}

	return closeErr.Code, closeErr.Text, true
}

// extractNormalWebSocketClose reports whether an error is a normal websocket
// closure (`1000` or `1001`) and returns close details when present.
//
// Parameters:
//   - err: websocket read/write error.
//
// Returns:
//   - int: close code when available.
//   - string: close reason text.
//   - bool: true when the error is a normal closure.
func extractNormalWebSocketClose(err error) (int, string, bool) {
	closeCode, closeReason, hasClose := extractWebSocketCloseDetails(err)
	if !hasClose {
		return 0, "", false
	}
	if closeCode == websocket.CloseNormalClosure || closeCode == websocket.CloseGoingAway {
		return closeCode, closeReason, true
	}

	return closeCode, closeReason, false
}

// tryBuildWebSocketErrorResponse converts a websocket error event into a synthesized
// HTTP error response compatible with existing relay error handling.
//
// Parameters:
//   - message: raw websocket text payload.
//
// Returns:
//   - *http.Response: synthesized non-200 error response.
//   - bool: true if the payload is a websocket error event.
func tryBuildWebSocketErrorResponse(message []byte) (*http.Response, bool) {
	var payload responseAPIWSErrorPayload
	if err := json.Unmarshal(message, &payload); err != nil {
		return nil, false
	}
	if !strings.EqualFold(payload.Type, "error") {
		return nil, false
	}

	status := payload.Status
	if status <= 0 {
		status = http.StatusBadRequest
	}

	if payload.Error == nil {
		payload.Error = map[string]any{
			"message": "upstream websocket returned error",
			"type":    "invalid_request_error",
		}
	}

	body, err := json.Marshal(map[string]any{"error": payload.Error})
	if err != nil {
		body = []byte(`{"error":{"message":"upstream websocket returned error","type":"invalid_request_error"}}`)
	}

	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewReader(body)),
	}, true
}

// shouldFallbackToHTTPForWebSocketError reports whether a websocket error event
// should trigger transport fallback to HTTP instead of returning the synthesized
// error directly.
//
// Parameters:
//   - message: raw websocket text payload.
//
// Returns:
//   - bool: true when HTTP fallback should be used.
func shouldFallbackToHTTPForWebSocketError(message []byte) bool {
	return strings.EqualFold(readWebSocketErrorCode(message), wsErrorCodeConnectionLimitReached)
}

// readWebSocketErrorCode extracts the error.code field from a websocket error event.
//
// Parameters:
//   - message: raw websocket text payload.
//
// Returns:
//   - string: normalized error code, or empty string when unavailable.
func readWebSocketErrorCode(message []byte) string {
	errPayload := parseWebSocketErrorPayload(message)
	if errPayload == nil || errPayload.Error == nil {
		return ""
	}

	code, ok := errPayload.Error["code"].(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(code)
}

// readWebSocketErrorType extracts the error.type field from a websocket error event.
//
// Parameters:
//   - message: raw websocket text payload.
//
// Returns:
//   - string: normalized error type, or empty string when unavailable.
func readWebSocketErrorType(message []byte) string {
	errPayload := parseWebSocketErrorPayload(message)
	if errPayload == nil || errPayload.Error == nil {
		return ""
	}

	typ, ok := errPayload.Error["type"].(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(typ)
}

// parseWebSocketErrorPayload parses a websocket payload as an error event payload.
//
// Parameters:
//   - message: raw websocket text payload.
//
// Returns:
//   - *responseAPIWSErrorPayload: parsed payload when message is an error event.
func parseWebSocketErrorPayload(message []byte) *responseAPIWSErrorPayload {
	var payload responseAPIWSErrorPayload
	if err := json.Unmarshal(message, &payload); err != nil {
		return nil
	}

	if !strings.EqualFold(payload.Type, "error") {
		return nil
	}

	return &payload
}

// buildStreamingWebSocketHTTPResponse bridges websocket events to an SSE body that existing
// stream handlers can consume unchanged.
//
// Parameters:
//   - conn: connected upstream websocket.
//   - firstMessage: first already-read text event.
//
// Returns:
//   - *http.Response: synthetic 200 response with text/event-stream body.
func buildStreamingWebSocketHTTPResponse(conn *websocket.Conn, firstMessage []byte) *http.Response {
	reader, writer := io.Pipe()

	go func() {
		defer func() {
			_ = conn.Close()
			_ = writer.Close()
		}()

		writeWebSocketMessageAsSSE(writer, firstMessage)
		if isWebSocketResponseTerminalEvent(firstMessage) {
			writeSSEDone(writer)
			return
		}

		for {
			message, err := readNextWebSocketTextMessage(conn)
			if err != nil {
				if _, _, normalClosure := extractNormalWebSocketClose(err); normalClosure {
					writeSSEDone(writer)
					return
				}
				writeSSEDone(writer)
				return
			}

			writeWebSocketMessageAsSSE(writer, message)
			if isWebSocketResponseTerminalEvent(message) {
				writeSSEDone(writer)
				return
			}
		}
	}()

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: reader,
	}
}

// writeWebSocketMessageAsSSE writes one websocket JSON payload as one SSE data event.
//
// Parameters:
//   - writer: output stream.
//   - message: raw websocket text payload.
func writeWebSocketMessageAsSSE(writer *io.PipeWriter, message []byte) {
	if len(bytes.TrimSpace(message)) == 0 {
		return
	}
	_, _ = writer.Write([]byte("data: "))
	_, _ = writer.Write(message)
	_, _ = writer.Write([]byte("\n\n"))
}

// writeSSEDone writes the terminal SSE [DONE] marker.
//
// Parameters:
//   - writer: output stream.
func writeSSEDone(writer *io.PipeWriter) {
	_, _ = writer.Write([]byte("data: [DONE]\n\n"))
}

// isWebSocketResponseTerminalEvent reports whether a websocket payload is terminal for
// a single response.create request.
//
// Parameters:
//   - message: raw websocket text payload.
//
// Returns:
//   - bool: true when the payload ends the response lifecycle.
func isWebSocketResponseTerminalEvent(message []byte) bool {
	var payload map[string]any
	if err := json.Unmarshal(message, &payload); err != nil {
		return false
	}

	eventType, _ := payload["type"].(string)
	switch eventType {
	case "response.completed", "response.failed", "error":
		return true
	}

	if object, _ := payload["object"].(string); object == "response" {
		if status, _ := payload["status"].(string); status == "completed" || status == "failed" {
			return true
		}
	}

	return false
}

// buildNonStreamingWebSocketHTTPResponse reads websocket events until completion and
// returns the final response JSON as a synthesized HTTP response.
//
// Parameters:
//   - conn: connected upstream websocket.
//   - firstMessage: first already-read text event.
//
// Returns:
//   - *http.Response: synthetic 200 JSON response with final response object body.
//   - error: when no final response can be reconstructed.
func buildNonStreamingWebSocketHTTPResponse(conn *websocket.Conn, firstMessage []byte) (*http.Response, error) {
	defer func() { _ = conn.Close() }()

	if errResp, ok := tryBuildWebSocketErrorResponse(firstMessage); ok {
		return errResp, nil
	}

	finalResponse, done := extractFinalResponseFromWebSocketMessage(firstMessage)
	if !done {
		for {
			message, err := readNextWebSocketTextMessage(conn)
			if err != nil {
				return nil, errors.Wrap(err, "read websocket response event")
			}
			if errResp, ok := tryBuildWebSocketErrorResponse(message); ok {
				return errResp, nil
			}
			if candidate, terminal := extractFinalResponseFromWebSocketMessage(message); candidate != nil {
				finalResponse = candidate
				if terminal {
					break
				}
			}
			if isWebSocketResponseTerminalEvent(message) {
				break
			}
		}
	}

	if finalResponse == nil {
		return nil, errors.New("response websocket completed without final response payload")
	}

	body, err := json.Marshal(finalResponse)
	if err != nil {
		return nil, errors.Wrap(err, "marshal final response websocket payload")
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewReader(body)),
	}, nil
}

// extractFinalResponseFromWebSocketMessage extracts response objects from websocket events.
//
// Parameters:
//   - message: websocket JSON payload.
//
// Returns:
//   - *ResponseAPIResponse: extracted response when present.
//   - bool: true when this message is terminal for the current response.
func extractFinalResponseFromWebSocketMessage(message []byte) (*ResponseAPIResponse, bool) {
	fullResponse, streamEvent, err := ParseResponseAPIStreamEvent(message)
	if err != nil {
		return nil, false
	}
	if fullResponse != nil {
		terminal := fullResponse.Status == "completed" || fullResponse.Status == "failed"
		return fullResponse, terminal
	}
	if streamEvent == nil {
		return nil, false
	}
	if streamEvent.Response != nil {
		terminal := strings.EqualFold(streamEvent.Type, "response.completed") ||
			strings.EqualFold(streamEvent.Type, "response.failed") ||
			streamEvent.Response.Status == "completed" ||
			streamEvent.Response.Status == "failed"
		return streamEvent.Response, terminal
	}
	return nil, strings.EqualFold(streamEvent.Type, "response.completed") || strings.EqualFold(streamEvent.Type, "response.failed")
}
